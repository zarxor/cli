// Package service manages background services installed by jb.
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zarxor/cli/internal/runner"
)

// Action is a lifecycle operation supported by a managed service.
type Action string

const (
	Install   Action = "install"
	Update    Action = "update"
	Status    Action = "status"
	Uninstall Action = "uninstall"
)

// Config describes the user account that owns a user-level service. Home and
// InvokingUser refer to the non-root user even when jb itself was launched via
// sudo.
type Config struct {
	Platform     string
	Home         string
	Root         bool
	InvokingUser string
	InvokingUID  int
}

// Result contains the command used for an operation and any output emitted by
// the wrapped service CLI.
type Result struct {
	Command string
	Output  string
	DryRun  bool
}

// T3CodeManager delegates service installation to the official T3 Code CLI.
// T3 Code owns the systemd unit, pinned runtime, launcher, and state files;
// jb only supplies the correct user environment and invokes that lifecycle.
type T3CodeManager struct {
	runner runner.Runner
	config Config
}

// NewT3CodeManager creates a T3 Code service manager.
func NewT3CodeManager(commandRunner runner.Runner, config Config) *T3CodeManager {
	if config.Platform == "" {
		config.Platform = runtime.GOOS
	}
	return &T3CodeManager{runner: commandRunner, config: config}
}

// Run installs, updates, reports, or removes the T3 Code background service.
// A dry run still resolves the command and user environment but does not
// invoke npx or mutate the host.
func (m *T3CodeManager) Run(ctx context.Context, action Action, baseDir string, dryRun bool) (Result, error) {
	command, err := m.command(ctx, action, baseDir)
	if err != nil {
		return Result{}, err
	}
	result := Result{Command: command.display(), DryRun: dryRun}
	if dryRun {
		return result, nil
	}

	output, err := m.runner.Run(ctx, command.name, command.args...)
	result.Output = joinOutput(output.Stdout, output.Stderr)
	if err != nil {
		return result, fmt.Errorf("run T3 Code service %s: %w", action, err)
	}
	return result, nil
}

type commandSpec struct {
	name string
	args []string
}

func (c commandSpec) display() string {
	parts := make([]string, 0, len(c.args)+1)
	parts = append(parts, quoteArg(c.name))
	for _, arg := range c.args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func (m *T3CodeManager) command(ctx context.Context, action Action, baseDir string) (commandSpec, error) {
	switch action {
	case Install, Update, Status, Uninstall:
	default:
		return commandSpec{}, fmt.Errorf("unsupported T3 Code service action %q", action)
	}
	if m.config.Platform != "linux" {
		return commandSpec{}, fmt.Errorf("T3 Code background service requires Linux with systemd; detected %q", m.config.Platform)
	}
	if m.config.Home == "" {
		return commandSpec{}, fmt.Errorf("resolve the home directory for the T3 Code service user")
	}

	args := []string{"--yes", "t3@latest", "service", string(action)}
	if strings.TrimSpace(baseDir) != "" {
		args = append(args, "--base-dir", baseDir)
	}

	// jb installs Node through nvm on Linux. nvm-exec keeps this command
	// independent of the invoking shell and gives the systemd unit an
	// absolute Node runtime path through T3 Code's own installer.
	nvmDir := filepath.Join(m.config.Home, ".nvm")
	nvmExec := filepath.Join(nvmDir, "nvm-exec")
	if executable, err := m.runner.LookPath(ctx, nvmExec); err == nil {
		environment := []string{"NVM_DIR=" + nvmDir, "NODE_VERSION=lts/*"}
		return m.asUserCommand(environment, executable, append([]string{"npx"}, args...)), nil
	}

	npx, err := m.runner.LookPath(ctx, "npx")
	if err != nil {
		return commandSpec{}, fmt.Errorf("find npx or nvm for the T3 Code service: %w", err)
	}
	return m.asUserCommand(nil, npx, args), nil
}

func (m *T3CodeManager) asUserCommand(environment []string, executable string, args []string) commandSpec {
	envArgs := []string{"HOME=" + m.config.Home}
	if m.config.InvokingUID > 0 {
		envArgs = append(envArgs, "XDG_RUNTIME_DIR=/run/user/"+strconv.Itoa(m.config.InvokingUID))
	}
	envArgs = append(envArgs, environment...)
	envArgs = append(envArgs, executable)
	envArgs = append(envArgs, args...)
	if m.config.Root && m.config.InvokingUser != "" && m.config.InvokingUser != "root" {
		return commandSpec{
			name: "sudo",
			args: append([]string{"-H", "-u", m.config.InvokingUser, "env"}, envArgs...),
		}
	}
	return commandSpec{name: "env", args: envArgs}
}

func joinOutput(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if output := strings.TrimSpace(stdout); output != "" {
		parts = append(parts, output)
	}
	if output := strings.TrimSpace(stderr); output != "" {
		parts = append(parts, output)
	}
	return strings.Join(parts, "\n")
}

func quoteArg(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\'', '"', '\\', '$', '`', '*', '?', '[', ']', '(', ')', ';', '&', '|', '<', '>':
			return true
		default:
			return false
		}
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
