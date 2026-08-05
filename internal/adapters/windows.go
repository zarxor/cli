package adapters

import (
	"context"
	"fmt"
	"os"

	"github.com/zarxor/scripts/internal/detect"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

// WindowsAdapter installs native Windows tools. It intentionally does not
// reuse Linux shell or nvm paths: WSL is a separate platform with its own
// adapter.
type WindowsAdapter struct {
	runner    runner.Runner
	elevation runner.Elevation
}

type windowsToolSource struct {
	executable string
	version    []string
	packageID  string
	installer  string
	system     bool
}

// windowsSources is the one source of truth for WinGet identities and
// official installer endpoints used by detection, installation, and updates.
var windowsSources = map[tools.ToolID]windowsToolSource{
	profile.Git:           {executable: "git", version: []string{"--version"}, packageID: "Git.Git", system: true},
	profile.GitHubCLI:     {executable: "gh", version: []string{"--version"}, packageID: "GitHub.cli", system: true},
	profile.Docker:        {executable: "docker", version: []string{"--version"}, packageID: "Docker.DockerDesktop", system: true},
	profile.DockerBuildx:  {executable: "docker", version: []string{"buildx", "version"}, packageID: "Docker.DockerDesktop", system: true},
	profile.DockerCompose: {executable: "docker", version: []string{"compose", "version"}, packageID: "Docker.DockerDesktop", system: true},
	profile.NVM:           {executable: "nvm", version: []string{"version"}, packageID: "CoreyButler.NVMforWindows", system: true},
	profile.Node:          {executable: "node", version: []string{"--version"}},
	profile.NPM:           {executable: "npm", version: []string{"--version"}},
	profile.Corepack:      {executable: "corepack", version: []string{"--version"}},
	profile.PNPM:          {executable: "pnpm", version: []string{"--version"}},
	profile.Yarn:          {executable: "yarn", version: []string{"--version"}},
	profile.Codex:         {executable: "codex", version: []string{"--version"}},
	profile.Bun:           {executable: "bun", version: []string{"--version"}, installer: "https://bun.sh/install.ps1"},
}

// NewWindowsAdapter creates the native Windows adapter. A nil elevation uses
// PowerShell's RunAs verb, while tests and callers can inject their own
// elevation boundary.
func NewWindowsAdapter(commandRunner runner.Runner, elevation runner.Elevation) Adapter {
	if elevation == nil {
		elevation = windowsElevation{runner: commandRunner}
	}
	return &WindowsAdapter{runner: commandRunner, elevation: elevation}
}

func (a *WindowsAdapter) Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return detect.Detection{}, a.unsupported(tool)
	}

	detection := detect.Detection{}
	if _, err := a.runner.LookPath(ctx, source.executable); err == nil {
		result, err := a.runner.Run(ctx, source.executable, source.version...)
		if err != nil {
			return detection, err
		}
		detection.Installed = true
		detection.Current = detect.ParseVersion(result.Stdout, result.Stderr)
	}
	if source.packageID != "" {
		result, err := a.runner.Run(ctx, "winget", "show", "--id", source.packageID, "--exact")
		if err == nil {
			detection.Candidate = labeledValue(result.Stdout, "Version")
		}
	}
	return detection, nil
}

func (a *WindowsAdapter) Install(ctx context.Context, tool tools.Tool) error {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	if source.packageID != "" {
		return a.winGet(ctx, "install", source)
	}
	return a.installUserTool(ctx, tool, source)
}

func (a *WindowsAdapter) Update(ctx context.Context, tool tools.Tool) error {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	detection, err := a.Detect(ctx, tool)
	if err != nil {
		return err
	}
	if !detection.Installed {
		return nil
	}
	if source.packageID != "" {
		return a.winGet(ctx, "upgrade", source)
	}
	return a.updateUserTool(ctx, tool, source)
}

func (a *WindowsAdapter) Verify(ctx context.Context, tool tools.Tool) error {
	detection, err := a.Detect(ctx, tool)
	if err != nil {
		return err
	}
	if !detection.Installed || detection.Current == "" {
		return fmt.Errorf("%s is not available after installation", tool.Name)
	}
	return nil
}

func (a *WindowsAdapter) winGet(ctx context.Context, action string, source windowsToolSource) error {
	args := []string{action, "--id", source.packageID, "--exact", "--accept-package-agreements", "--accept-source-agreements"}
	if !source.system {
		return a.run(ctx, "winget", args...)
	}
	_, err := a.elevation.RunElevated(ctx, "winget", args...)
	return err
}

func (a *WindowsAdapter) installUserTool(ctx context.Context, tool tools.Tool, source windowsToolSource) error {
	switch tool.ID {
	case profile.Node:
		return a.runNVM(ctx)
	case profile.NPM:
		return a.run(ctx, "npm", "install", "--global", "npm@latest")
	case profile.Corepack:
		if err := a.run(ctx, "npm", "install", "--global", "corepack@latest"); err != nil {
			return err
		}
		return a.run(ctx, "corepack", "enable")
	case profile.PNPM:
		return a.run(ctx, "corepack", "prepare", "pnpm@latest", "--activate")
	case profile.Yarn:
		return a.run(ctx, "corepack", "prepare", "yarn@stable", "--activate")
	case profile.Codex:
		return a.run(ctx, "npm", "install", "--global", "@openai/codex")
	case profile.Bun:
		return a.runPowerShellInstaller(ctx, source.installer)
	default:
		return a.unsupported(tool)
	}
}

func (a *WindowsAdapter) updateUserTool(ctx context.Context, tool tools.Tool, source windowsToolSource) error {
	switch tool.ID {
	case profile.Bun:
		return a.run(ctx, "bun", "upgrade", "--stable")
	case profile.Node:
		return a.runNVM(ctx)
	default:
		return a.installUserTool(ctx, tool, source)
	}
}

// runPowerShellInstaller keeps the PowerShell program static and provides the
// URL as a separate argument rather than interpolating it into shell text.
func (a *WindowsAdapter) runPowerShellInstaller(ctx context.Context, installerURL string) error {
	if installerURL == "" {
		return fmt.Errorf("missing Windows installer URL")
	}
	helper, err := os.CreateTemp("", "jb-windows-install-*.ps1")
	if err != nil {
		return err
	}
	path := helper.Name()
	defer os.Remove(path)
	if _, err := helper.WriteString("param([string]$InstallerURL)\nInvoke-RestMethod -Uri $InstallerURL | Invoke-Expression\n"); err != nil {
		_ = helper.Close()
		return err
	}
	if err := helper.Close(); err != nil {
		return err
	}
	return a.run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path, installerURL)
}

func (a *WindowsAdapter) run(ctx context.Context, command string, args ...string) error {
	_, err := a.runner.Run(ctx, command, args...)
	return err
}

func (a *WindowsAdapter) runNVM(ctx context.Context) error {
	for _, args := range [][]string{{"install", "lts"}, {"use", "lts"}} {
		if _, err := a.elevation.RunElevated(ctx, "nvm", args...); err != nil {
			return err
		}
	}
	return nil
}

func (a *WindowsAdapter) unsupported(tool tools.Tool) error {
	return fmt.Errorf("Windows adapter does not support an installation path for %q", tool.ID)
}

// windowsElevation requests an elevated native process with a static
// PowerShell helper. The target command and every argument remain separate
// -File arguments, avoiding an interpolated command line.
type windowsElevation struct{ runner runner.Runner }

func (e windowsElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	helper, err := os.CreateTemp("", "jb-windows-elevate-*.ps1")
	if err != nil {
		return runner.Result{}, err
	}
	path := helper.Name()
	defer os.Remove(path)
	const runAs = "param(\n\t[Parameter(Mandatory=$true, Position=0)][string]$FilePath,\n\t[Parameter(ValueFromRemainingArguments=$true)][string[]]$ArgumentList\n)\n$process = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -Verb RunAs -Wait -PassThru\nexit $process.ExitCode\n"
	if _, err := helper.WriteString(runAs); err != nil {
		_ = helper.Close()
		return runner.Result{}, err
	}
	if err := helper.Close(); err != nil {
		return runner.Result{}, err
	}
	return e.runner.Run(ctx, "powershell.exe", append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path, command}, args...)...)
}

var _ Adapter = (*WindowsAdapter)(nil)
