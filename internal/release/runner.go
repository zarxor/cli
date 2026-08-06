package release

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner isolates external command discovery and execution.
type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

// ExecRunner executes commands on the host system.
type ExecRunner struct{}

func (ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return string(output), fmt.Errorf("run %s %s: %w", name, strings.Join(args, " "), err)
		}
		return string(output), fmt.Errorf("run %s %s: %w: %s", name, strings.Join(args, " "), err, message)
	}
	return string(output), nil
}
