package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
	"github.com/zarxor/cli/internal/tools"
)

// WindowsAdapter installs native Windows tools. It intentionally does not
// reuse Linux shell or nvm paths: WSL is a separate platform with its own
// adapter.
type WindowsAdapter struct {
	runner                   runner.Runner
	elevation                runner.Elevation
	config                   WindowsConfig
	converged                map[string]struct{}
	dockerDesktopVersionsMu  sync.Mutex
	dockerDesktopVersionsSet bool
	dockerDesktopCurrent     string
	dockerDesktopCandidate   string
	wingetPackagesMu         sync.Mutex
	wingetPackages           map[string]wingetPackageInfo
}

type WindowsConfig struct {
	ProgramFiles string
	NVMHome      string
	NVMSymlink   string
}

type windowsToolSource struct {
	executable string
	version    []string
	packageID  string
	system     bool
}

type wingetPackageInfo struct {
	status  ownershipStatus
	current string
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
	profile.Bun:           {executable: "bun", version: []string{"--version"}},
}

// NewWindowsAdapter creates the native Windows adapter. A nil elevation uses
// PowerShell's RunAs verb, while tests and callers can inject their own
// elevation boundary.
func NewWindowsAdapter(commandRunner runner.Runner, elevation runner.Elevation, configs ...WindowsConfig) Adapter {
	if elevation == nil {
		elevation = windowsElevation{runner: commandRunner}
	}
	config := WindowsConfig{ProgramFiles: os.Getenv("ProgramFiles"), NVMHome: os.Getenv("NVM_HOME"), NVMSymlink: os.Getenv("NVM_SYMLINK")}
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.NVMHome == "" {
		config.NVMHome = filepath.Join(os.Getenv("APPDATA"), "nvm")
	}
	if config.NVMSymlink == "" {
		config.NVMSymlink = filepath.Join(config.ProgramFiles, "nodejs")
	}
	return &WindowsAdapter{
		runner: commandRunner, elevation: elevation, config: config,
		converged: make(map[string]struct{}), wingetPackages: make(map[string]wingetPackageInfo),
	}
}

func (a *WindowsAdapter) Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return detect.Detection{}, a.unsupported(tool)
	}

	detection, err := a.detectInstalled(ctx, tool, source)
	if err != nil {
		return detection, err
	}
	if source.packageID != "" {
		if isDockerTool(tool.ID) {
			current, candidate := a.dockerDesktopVersions(ctx)
			if current != "" {
				detection.Current = current
			}
			if candidate != "" {
				detection.Candidate = candidate
			}
		} else if a.wingetPackage(ctx, source.packageID).status != ownershipNotOwned {
			result, err := a.runner.Run(ctx, "winget", "show", "--id", source.packageID, "--exact")
			if err == nil {
				detection.Candidate = labeledValue(result.Stdout, "Version")
			}
		}
	} else if detection.Installed {
		if tool.ID == profile.Node && !a.nodeUsesNVM(ctx) {
			return detection, nil
		}
		candidate, err := a.userToolCandidate(ctx, tool.ID)
		if err != nil {
			return detection, nil
		}
		detection.Candidate = candidate
	}
	return detection, nil
}

func (a *WindowsAdapter) detectInstalled(ctx context.Context, tool tools.Tool, source windowsToolSource) (detect.Detection, error) {
	detection := detect.Detection{}
	if executable, resolveErr := a.resolveExecutable(ctx, source.executable); resolveErr == nil {
		result, runErr := a.runner.Run(ctx, executable, source.version...)
		if runErr != nil {
			if !expectedMissingComponent(tool.ID, result) && !expectedMissingComponentExecutable(source.executable, result) {
				return detection, runErr
			}
		} else {
			detection.Installed = true
			detection.Current = detect.ParseVersion(result.Stdout, result.Stderr)
		}
	}
	return detection, nil
}

func (a *WindowsAdapter) dockerDesktopVersions(ctx context.Context) (string, string) {
	a.dockerDesktopVersionsMu.Lock()
	defer a.dockerDesktopVersionsMu.Unlock()
	if a.dockerDesktopVersionsSet {
		return a.dockerDesktopCurrent, a.dockerDesktopCandidate
	}
	a.dockerDesktopVersionsSet = true
	packageID := windowsSources[profile.Docker].packageID
	packageInfo := a.wingetPackage(ctx, packageID)
	if packageInfo.status == ownershipNotOwned {
		return "", ""
	}
	a.dockerDesktopCurrent = packageInfo.current
	if result, err := a.runner.Run(ctx, "winget", "show", "--id", packageID, "--exact"); err == nil {
		a.dockerDesktopCandidate = labeledValue(result.Stdout, "Version")
	}
	return a.dockerDesktopCurrent, a.dockerDesktopCandidate
}

func (a *WindowsAdapter) wingetPackage(ctx context.Context, packageID string) wingetPackageInfo {
	a.wingetPackagesMu.Lock()
	defer a.wingetPackagesMu.Unlock()
	if packageInfo, ok := a.wingetPackages[packageID]; ok {
		return packageInfo
	}

	result, err := a.runner.Run(ctx, "winget", "list", "--id", packageID, "--exact", "--details")
	packageInfo := parseWinGetPackageInfo(packageID, result, err)
	a.wingetPackages[packageID] = packageInfo
	return packageInfo
}

func parseWinGetPackageInfo(packageID string, result runner.Result, err error) wingetPackageInfo {
	output := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	if err != nil {
		return wingetPackageInfo{status: ownershipNotOwned}
	}
	if output == "" {
		return wingetPackageInfo{status: ownershipUnknown}
	}
	current := labeledValue(result.Stdout, "Version")
	if current == "" {
		current = labeledValue(result.Stderr, "Version")
	}
	if current != "" || containsOutputToken(output, packageID) {
		return wingetPackageInfo{status: ownershipOwned, current: current}
	}
	return wingetPackageInfo{status: ownershipNotOwned}
}

func containsOutputToken(output, expected string) bool {
	for _, field := range strings.FieldsFunc(output, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '|' || r == ':' || r == ','
	}) {
		if strings.EqualFold(strings.Trim(field, "\"'"), expected) {
			return true
		}
	}
	return false
}

func (a *WindowsAdapter) userToolCandidate(ctx context.Context, id tools.ToolID) (string, error) {
	if id == profile.Node {
		nvm, err := a.resolveExecutable(ctx, "nvm")
		if err != nil {
			return "", err
		}
		result, err := a.runner.Run(ctx, nvm, "list", "available")
		if err != nil {
			return "", err
		}
		matches := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).FindAllString(result.Stdout, -1)
		if len(matches) < 2 {
			return "", fmt.Errorf("nvm release metadata contains no LTS version")
		}
		return matches[1], nil
	}
	packageName, ok := map[tools.ToolID]string{
		profile.NPM: "npm", profile.Corepack: "corepack", profile.PNPM: "pnpm",
		profile.Yarn: "@yarnpkg/cli-dist", profile.Codex: "@openai/codex", profile.Bun: "bun",
	}[id]
	if !ok {
		return "", nil
	}
	npm, err := a.resolveExecutable(ctx, "npm")
	if err != nil {
		return "", err
	}
	result, err := a.runner.Run(ctx, npm, "view", packageName, "version")
	if err != nil {
		return "", err
	}
	return detect.ParseVersion(result.Stdout, result.Stderr), nil
}

func (a *WindowsAdapter) nodeUsesNVM(ctx context.Context) bool {
	node, err := a.resolveExecutable(ctx, "node")
	if err != nil {
		return false
	}
	if a.config.NVMSymlink == "" {
		return false
	}
	if !filepath.IsAbs(a.config.NVMSymlink) {
		// Test and embedded environments may not expose ProgramFiles. The
		// conventional NVM for Windows link directory is still identifiable.
		return strings.EqualFold(filepath.Base(filepath.Dir(node)), "nodejs")
	}
	relative, err := filepath.Rel(filepath.Clean(a.config.NVMSymlink), filepath.Clean(node))
	if err != nil {
		return false
	}
	relative = strings.ToLower(relative)
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (a *WindowsAdapter) resolveExecutable(ctx context.Context, name string) (string, error) {
	if executable, err := a.runner.LookPath(ctx, name); err == nil {
		return executable, nil
	}
	for _, candidate := range a.executableCandidates(name) {
		if candidate == "" {
			continue
		}
		if executable, err := a.runner.LookPath(ctx, candidate); err == nil {
			return executable, nil
		}
	}
	return "", fmt.Errorf("executable %q not found in process PATH or provider locations", name)
}

func (a *WindowsAdapter) executableCandidates(name string) []string {
	switch name {
	case "git":
		return []string{filepath.Join(a.config.ProgramFiles, "Git", "cmd", "git.exe")}
	case "gh":
		return []string{filepath.Join(a.config.ProgramFiles, "GitHub CLI", "gh.exe")}
	case "docker":
		return []string{filepath.Join(a.config.ProgramFiles, "Docker", "Docker", "resources", "bin", "docker.exe")}
	case "nvm":
		return []string{filepath.Join(a.config.NVMHome, "nvm.exe")}
	case "node":
		return []string{filepath.Join(a.config.NVMSymlink, "node.exe")}
	case "npm", "corepack", "pnpm", "yarn", "codex":
		return []string{filepath.Join(a.config.NVMSymlink, name+".cmd")}
	case "bun":
		return []string{filepath.Join(a.config.NVMSymlink, "bun.exe"), filepath.Join(a.config.NVMSymlink, "bun.cmd")}
	default:
		return nil
	}
}

func (a *WindowsAdapter) Install(ctx context.Context, tool tools.Tool) error {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	if source.packageID != "" {
		return a.winGet(ctx, "install", source)
	}
	return a.installUserTool(ctx, tool)
}

func (a *WindowsAdapter) Update(ctx context.Context, tool tools.Tool) error {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	detection, err := a.detectInstalled(ctx, tool, source)
	if err != nil {
		return err
	}
	if !detection.Installed {
		return nil
	}
	if source.packageID != "" {
		if a.wingetPackage(ctx, source.packageID).status == ownershipNotOwned {
			return fmt.Errorf("%s is not managed by WinGet; refusing to update a different installation", tool.Name)
		}
		return a.winGet(ctx, "upgrade", source)
	}
	if tool.ID == profile.Node && !a.nodeUsesNVM(ctx) {
		return fmt.Errorf("%s is not managed by nvm; refusing to update a different installation", tool.Name)
	}
	return a.updateUserTool(ctx, tool)
}

func (a *WindowsAdapter) Verify(ctx context.Context, tool tools.Tool) error {
	source, ok := windowsSources[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	detection, err := a.detectInstalled(ctx, tool, source)
	if err != nil {
		return err
	}
	if !detection.Installed || detection.Current == "" {
		return fmt.Errorf("%s is not available after installation", tool.Name)
	}
	return nil
}

func (a *WindowsAdapter) winGet(ctx context.Context, action string, source windowsToolSource) error {
	if _, ok := a.converged[source.packageID]; ok {
		return nil
	}
	force := false
	if source.packageID == windowsSources[profile.Docker].packageID {
		var err error
		force, err = a.dockerDesktopNeedsRepair(ctx)
		if err != nil {
			return err
		}
	}
	if force {
		action = "install"
	}
	args := []string{action, "--id", source.packageID, "--exact", "--accept-package-agreements", "--accept-source-agreements"}
	if force {
		args = append(args, "--force")
	}
	var err error
	if !source.system {
		err = a.run(ctx, "winget", args...)
	} else {
		_, err = a.elevation.RunElevated(ctx, "winget", args...)
	}
	if err == nil {
		a.converged[source.packageID] = struct{}{}
		a.dockerDesktopVersionsMu.Lock()
		a.dockerDesktopVersionsSet = false
		a.dockerDesktopCurrent = ""
		a.dockerDesktopCandidate = ""
		a.dockerDesktopVersionsMu.Unlock()
		a.wingetPackagesMu.Lock()
		delete(a.wingetPackages, source.packageID)
		a.wingetPackagesMu.Unlock()
	}
	return err
}

func (a *WindowsAdapter) dockerDesktopNeedsRepair(ctx context.Context) (bool, error) {
	executable, err := a.resolveExecutable(ctx, windowsSources[profile.Docker].executable)
	if err != nil {
		return false, nil
	}
	if _, err := a.runner.Run(ctx, executable, windowsSources[profile.Docker].version...); err != nil {
		return false, err
	}
	for _, id := range []tools.ToolID{profile.DockerBuildx, profile.DockerCompose} {
		result, err := a.runner.Run(ctx, executable, windowsSources[id].version...)
		if err == nil {
			continue
		}
		if expectedMissingComponent(id, result) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (a *WindowsAdapter) installUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Node:
		return a.runNVM(ctx)
	case profile.NPM:
		return a.runResolved(ctx, "npm", "install", "--global", "npm@latest")
	case profile.Corepack:
		if err := a.runResolved(ctx, "npm", "install", "--global", "corepack@latest"); err != nil {
			return err
		}
		return a.runResolved(ctx, "corepack", "enable")
	case profile.PNPM:
		return a.runResolved(ctx, "corepack", "prepare", "pnpm@latest", "--activate")
	case profile.Yarn:
		return a.runResolved(ctx, "corepack", "prepare", "yarn@stable", "--activate")
	case profile.Codex:
		return a.runResolved(ctx, "npm", "install", "--global", "@openai/codex@latest")
	case profile.Bun:
		return a.installBun(ctx)
	default:
		return a.unsupported(tool)
	}
}

func (a *WindowsAdapter) updateUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Bun:
		return a.installBun(ctx)
	case profile.Node:
		return a.runNVM(ctx)
	default:
		return a.installUserTool(ctx, tool)
	}
}

func (a *WindowsAdapter) installBun(ctx context.Context) error {
	// Keep Bun in the active NVM for Windows Node directory even when npm has
	// an inherited global prefix, and force the postinstall and command shim.
	if err := a.runResolved(ctx, "npm", "install", "--global", "--prefix", a.config.NVMSymlink, "--ignore-scripts=false", "--bin-links=true", "--allow-scripts=bun", "bun@latest"); err != nil {
		return err
	}
	return a.repairBunPostinstall(ctx)
}

func (a *WindowsAdapter) repairBunPostinstall(ctx context.Context) error {
	result, err := a.runResolvedCommand(ctx, "bun", "--version")
	if err == nil {
		if detect.ParseVersion(result.Stdout, result.Stderr) == "" {
			return fmt.Errorf("Bun returned no version after npm installation")
		}
		return nil
	}
	if !expectedBunPostinstallFailure(result) {
		return fmt.Errorf("verify Bun after npm installation: %w", err)
	}
	if err := a.runResolved(ctx, "npm", "explore", "--global", "--prefix", a.config.NVMSymlink, "bun", "--", "node", "install.js"); err != nil {
		return fmt.Errorf("repair Bun postinstall: %w", err)
	}
	result, err = a.runResolvedCommand(ctx, "bun", "--version")
	if err != nil {
		return fmt.Errorf("verify Bun after postinstall repair: %w", err)
	}
	if detect.ParseVersion(result.Stdout, result.Stderr) == "" {
		return fmt.Errorf("Bun returned no version after postinstall repair")
	}
	return nil
}

func (a *WindowsAdapter) run(ctx context.Context, command string, args ...string) error {
	_, err := a.runner.Run(ctx, command, args...)
	return err
}

func (a *WindowsAdapter) runResolved(ctx context.Context, name string, args ...string) error {
	_, err := a.runResolvedCommand(ctx, name, args...)
	return err
}

func (a *WindowsAdapter) runResolvedCommand(ctx context.Context, name string, args ...string) (runner.Result, error) {
	executable, err := a.resolveExecutable(ctx, name)
	if err != nil {
		return runner.Result{}, err
	}
	return a.runner.Run(ctx, executable, args...)
}

func (a *WindowsAdapter) runNVM(ctx context.Context) error {
	nvm, err := a.resolveExecutable(ctx, "nvm")
	if err != nil {
		return err
	}
	for _, args := range [][]string{{"install", "lts"}, {"use", "lts"}} {
		if _, err := a.elevation.RunElevated(ctx, nvm, args...); err != nil {
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
