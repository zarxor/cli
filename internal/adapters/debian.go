// Package adapters provides platform-specific tool operations.
package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zarxor/scripts/internal/detect"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

// Adapter is the platform boundary used by installation planning.
type Adapter interface {
	Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error)
	Install(ctx context.Context, tool tools.Tool) error
	Update(ctx context.Context, tool tools.Tool) error
	Verify(ctx context.Context, tool tools.Tool) error
}

// LinuxConfig supplies host facts which platform detection already knows and
// keeps adapter fixtures independent from the machine running the tests.
type LinuxConfig struct {
	Root         bool
	Home         string
	TempDir      string
	Distribution string
	Codename     string
	Architecture string
}

// DockerConflictCandidates are packages Docker documents as conflicting with
// its Debian/Ubuntu Engine packages. They are queried before any removal.
var DockerConflictCandidates = []string{
	"docker.io", "docker-compose", "docker-compose-v2", "docker-doc",
	"docker-buildx", "podman-docker", "containerd", "runc",
}

const (
	githubCLIKeyURL = "https://cli.github.com/packages/githubcli-archive-keyring.gpg"
	codexInstaller  = "https://chatgpt.com/codex/install.sh"
	nvmInstaller    = "https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh"
	bunInstaller    = "https://bun.sh/install"
)

type linuxAdapter struct {
	runner    runner.Runner
	elevation runner.Elevation
	config    LinuxConfig
}

// DebianAdapter installs tools through apt and the supported upstream apt
// repositories.
type DebianAdapter struct{ linuxAdapter }

// NewDebianAdapter builds an adapter. The optional config makes live host
// discovery a caller concern while keeping deterministic defaults for tests.
func NewDebianAdapter(commandRunner runner.Runner, elevation runner.Elevation, configs ...LinuxConfig) Adapter {
	return &DebianAdapter{linuxAdapter: newLinuxAdapter(commandRunner, elevation, configs...)}
}

func newLinuxAdapter(commandRunner runner.Runner, elevation runner.Elevation, configs ...LinuxConfig) linuxAdapter {
	config := LinuxConfig{}
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.Home == "" {
		config.Home, _ = os.UserHomeDir()
	}
	if config.TempDir == "" {
		config.TempDir = os.TempDir()
	}
	if config.Distribution == "" {
		config.Distribution = "debian"
	}
	if config.Architecture == "" {
		config.Architecture = debianArchitecture(runtime.GOARCH)
	}
	return linuxAdapter{runner: commandRunner, elevation: elevation, config: config}
}

func debianArchitecture(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

func (a *DebianAdapter) Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	detection, err := a.detect(ctx, tool)
	if err != nil || !detection.Installed {
		return detection, err
	}
	packages, ok := debianPackages(tool.ID)
	if !ok || len(packages) == 0 {
		return detection, nil
	}
	result, err := a.runner.Run(ctx, "apt-cache", "policy", packages[0])
	if err != nil {
		return detection, nil
	}
	detection.Candidate = labeledValue(result.Stdout, "Candidate")
	return detection, nil
}

func (a *DebianAdapter) Install(ctx context.Context, tool tools.Tool) error {
	if packages, ok := debianPackages(tool.ID); ok {
		if tool.ID == profile.GitHubCLI {
			if err := a.configureGitHubCLI(ctx); err != nil {
				return err
			}
		}
		if isDockerTool(tool.ID) {
			if tool.ID == profile.Docker {
				if err := a.removeDockerConflicts(ctx); err != nil {
					return err
				}
			}
			if err := a.configureDocker(ctx); err != nil {
				return err
			}
		}
		if err := a.system(ctx, "apt-get", "update"); err != nil {
			return err
		}
		if err := a.system(ctx, "apt-get", append([]string{"install", "-y"}, packages...)...); err != nil {
			return err
		}
		if tool.ID == profile.Docker {
			return a.system(ctx, "systemctl", "enable", "--now", "docker.service")
		}
		return nil
	}
	if tool.ID == profile.Bun {
		if err := a.system(ctx, "apt-get", "install", "-y", "unzip"); err != nil {
			return err
		}
	}
	return a.installUserTool(ctx, tool)
}

func (a *DebianAdapter) Update(ctx context.Context, tool tools.Tool) error {
	if packages, ok := debianPackages(tool.ID); ok {
		if tool.ID == profile.GitHubCLI {
			if err := a.configureGitHubCLI(ctx); err != nil {
				return err
			}
		}
		if isDockerTool(tool.ID) {
			if err := a.configureDocker(ctx); err != nil {
				return err
			}
		}
		if err := a.system(ctx, "apt-get", "update"); err != nil {
			return err
		}
		return a.system(ctx, "apt-get", append([]string{"install", "--only-upgrade", "-y"}, packages...)...)
	}
	return a.updateUserTool(ctx, tool)
}

func labeledValue(output, label string) string {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == label {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (a *DebianAdapter) Verify(ctx context.Context, tool tools.Tool) error {
	return a.verify(ctx, tool)
}

func debianPackages(id tools.ToolID) ([]string, bool) {
	switch id {
	case profile.Git:
		return []string{"git"}, true
	case profile.GitHubCLI:
		return []string{"gh"}, true
	case profile.Docker:
		return []string{"docker-ce", "docker-ce-cli", "containerd.io"}, true
	case profile.DockerBuildx:
		return []string{"docker-buildx-plugin"}, true
	case profile.DockerCompose:
		return []string{"docker-compose-plugin"}, true
	default:
		return nil, false
	}
}

func isDockerTool(id tools.ToolID) bool {
	return id == profile.Docker || id == profile.DockerBuildx || id == profile.DockerCompose
}

func (a *DebianAdapter) configureGitHubCLI(ctx context.Context) error {
	if err := a.system(ctx, "install", "-m", "0755", "-d", "/etc/apt/keyrings", "/etc/apt/sources.list.d"); err != nil {
		return err
	}
	key := filepath.Join(a.config.TempDir, "githubcli-archive-keyring.gpg")
	if err := a.download(ctx, githubCLIKeyURL, key); err != nil {
		return err
	}
	if err := a.system(ctx, "install", "-m", "0644", key, "/etc/apt/keyrings/githubcli-archive-keyring.gpg"); err != nil {
		return err
	}
	source := filepath.Join(a.config.TempDir, "github-cli.list")
	content := fmt.Sprintf("deb [arch=%s signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n", a.config.Architecture)
	if err := writeTemporary(source, content); err != nil {
		return err
	}
	return a.system(ctx, "install", "-m", "0644", source, "/etc/apt/sources.list.d/github-cli.list")
}

func (a *DebianAdapter) configureDocker(ctx context.Context) error {
	if a.config.Codename == "" {
		return fmt.Errorf("Docker apt repository requires a distribution codename")
	}
	if a.config.Distribution != "debian" && a.config.Distribution != "ubuntu" {
		return fmt.Errorf("Docker apt repository does not support distribution %q", a.config.Distribution)
	}
	if err := a.system(ctx, "install", "-m", "0755", "-d", "/etc/apt/keyrings", "/etc/apt/sources.list.d"); err != nil {
		return err
	}
	key := filepath.Join(a.config.TempDir, "docker.asc")
	keyURL := "https://download.docker.com/linux/" + a.config.Distribution + "/gpg"
	if err := a.download(ctx, keyURL, key); err != nil {
		return err
	}
	if err := a.system(ctx, "install", "-m", "0644", key, "/etc/apt/keyrings/docker.asc"); err != nil {
		return err
	}
	source := filepath.Join(a.config.TempDir, "docker.sources")
	content := fmt.Sprintf("Types: deb\nURIs: https://download.docker.com/linux/%s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n", a.config.Distribution, a.config.Codename, a.config.Architecture)
	if err := writeTemporary(source, content); err != nil {
		return err
	}
	return a.system(ctx, "install", "-m", "0644", source, "/etc/apt/sources.list.d/docker.sources")
}

func (a *DebianAdapter) removeDockerConflicts(ctx context.Context) error {
	installed := make([]string, 0, len(DockerConflictCandidates))
	for _, candidate := range DockerConflictCandidates {
		result, err := a.runner.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}", candidate)
		if err == nil && strings.TrimSpace(result.Stdout) == "installed" {
			installed = append(installed, candidate)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	return a.system(ctx, "apt-get", append([]string{"remove", "-y"}, installed...)...)
}

func (a linuxAdapter) system(ctx context.Context, command string, args ...string) error {
	if a.config.Root {
		_, err := a.runner.Run(ctx, command, args...)
		return err
	}
	_, err := a.runner.Run(ctx, "sudo", append([]string{command}, args...)...)
	return err
}

func (a linuxAdapter) download(ctx context.Context, url, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	_, err := a.runner.Run(ctx, "curl", "-fsSL", url, "-o", destination)
	return err
}

func writeTemporary(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}

func (a linuxAdapter) detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	command, args, err := a.versionCommand(tool.ID)
	if err != nil {
		return detect.Detection{}, err
	}
	if _, err := a.runner.LookPath(ctx, command); err != nil {
		return detect.Detection{Installed: false}, nil
	}
	result, err := a.runner.Run(ctx, command, args...)
	if err != nil {
		return detect.Detection{}, err
	}
	return detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}, nil
}

func (a linuxAdapter) verify(ctx context.Context, tool tools.Tool) error {
	detection, err := a.detect(ctx, tool)
	if err != nil {
		return err
	}
	if !detection.Installed || detection.Current == "" {
		return fmt.Errorf("%s is not available after installation", tool.Name)
	}
	return nil
}

func (a linuxAdapter) versionCommand(id tools.ToolID) (string, []string, error) {
	switch id {
	case profile.Git:
		return "git", []string{"--version"}, nil
	case profile.GitHubCLI:
		return "gh", []string{"--version"}, nil
	case profile.Docker:
		return "docker", []string{"--version"}, nil
	case profile.DockerBuildx:
		return "docker", []string{"buildx", "version"}, nil
	case profile.DockerCompose:
		return "docker", []string{"compose", "version"}, nil
	case profile.Codex:
		return "codex", []string{"--version"}, nil
	case profile.NVM:
		return filepath.Join(a.config.Home, ".nvm", "nvm-exec"), []string{"nvm", "--version"}, nil
	case profile.Node:
		return "node", []string{"--version"}, nil
	case profile.NPM:
		return "npm", []string{"--version"}, nil
	case profile.Corepack:
		return "corepack", []string{"--version"}, nil
	case profile.PNPM:
		return "pnpm", []string{"--version"}, nil
	case profile.Yarn:
		return "yarn", []string{"--version"}, nil
	case profile.Bun:
		return filepath.Join(a.config.Home, ".bun", "bin", "bun"), []string{"--version"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported tool %q", id)
	}
}

func (a linuxAdapter) installUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Codex:
		if err := a.runInstaller(ctx, "codex", codexInstaller, "env", "HOME="+a.config.Home, "sh"); err != nil {
			return err
		}
		return a.ensureProfileBlock("paths", `export PATH="$HOME/.local/bin:$PATH"`)
	case profile.NVM:
		return a.installNVM(ctx, nvmInstaller)
	case profile.Node:
		return a.runNVM(ctx, "install", "--lts", "--latest-npm")
	case profile.NPM:
		return a.userRun(ctx, "npm", "install", "--global", "npm@latest")
	case profile.Corepack:
		if err := a.userRun(ctx, "npm", "install", "--global", "corepack@latest"); err != nil {
			return err
		}
		return a.userRun(ctx, "corepack", "enable")
	case profile.PNPM:
		return a.userRun(ctx, "corepack", "prepare", "pnpm@latest", "--activate")
	case profile.Yarn:
		return a.userRun(ctx, "corepack", "prepare", "yarn@stable", "--activate")
	case profile.Bun:
		if err := a.runInstaller(ctx, "bun", bunInstaller, "env", "HOME="+a.config.Home, "BUN_INSTALL="+filepath.Join(a.config.Home, ".bun"), "bash"); err != nil {
			return err
		}
		return a.ensureProfileBlock("bun", "export BUN_INSTALL=\"$HOME/.bun\"\nexport PATH=\"$BUN_INSTALL/bin:$PATH\"")
	default:
		return fmt.Errorf("unsupported tool %q", tool.ID)
	}
}

func (a linuxAdapter) updateUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Bun:
		return a.userRun(ctx, filepath.Join(a.config.Home, ".bun", "bin", "bun"), "upgrade", "--stable")
	case profile.NVM:
		installerURL, err := a.latestNVMInstaller(ctx)
		if err != nil {
			return err
		}
		return a.installNVM(ctx, installerURL)
	case profile.Node:
		return a.runNVM(ctx, "install", "--lts", "--latest-npm")
	default:
		return a.installUserTool(ctx, tool)
	}
}

func (a linuxAdapter) installNVM(ctx context.Context, installerURL string) error {
	if err := a.runInstaller(ctx, "nvm", installerURL, "env", "HOME="+a.config.Home, "PROFILE=/dev/null", "NVM_DIR="+filepath.Join(a.config.Home, ".nvm"), "bash"); err != nil {
		return err
	}
	return a.ensureProfileBlock("nvm", "export NVM_DIR=\"$HOME/.nvm\"\n[ -s \"$NVM_DIR/nvm.sh\" ] && \\. \"$NVM_DIR/nvm.sh\"\n[ -s \"$NVM_DIR/bash_completion\" ] && \\. \"$NVM_DIR/bash_completion\"")
}

func (a linuxAdapter) latestNVMInstaller(ctx context.Context) (string, error) {
	result, err := a.runner.Run(ctx, "git", "ls-remote", "--tags", "--refs", "https://github.com/nvm-sh/nvm.git", "v*")
	if err != nil {
		return "", err
	}
	latest := ""
	latestParts := [3]int{-1, -1, -1}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		parts, ok := stableNVMVersion(tag)
		if ok && newerVersion(parts, latestParts) {
			latest = tag
			latestParts = parts
		}
	}
	if latest == "" {
		return "", fmt.Errorf("could not resolve a stable nvm release")
	}
	return "https://raw.githubusercontent.com/nvm-sh/nvm/" + latest + "/install.sh", nil
}

func stableNVMVersion(tag string) ([3]int, bool) {
	var version [3]int
	values := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if !strings.HasPrefix(tag, "v") || len(values) != len(version) {
		return version, false
	}
	for index, value := range values {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return [3]int{}, false
		}
		version[index] = parsed
	}
	return version, true
}

func newerVersion(candidate, current [3]int) bool {
	for index := range candidate {
		if candidate[index] != current[index] {
			return candidate[index] > current[index]
		}
	}
	return false
}

func (a linuxAdapter) runInstaller(ctx context.Context, name, url, command string, prefixArgs ...string) error {
	installer := filepath.Join(a.config.TempDir, "jb-"+name+"-install.sh")
	if err := writeTemporary(installer, ""); err != nil {
		return err
	}
	defer os.Remove(installer)
	if err := a.download(ctx, url, installer); err != nil {
		return err
	}
	args := append(append([]string(nil), prefixArgs...), installer)
	_, err := a.runner.Run(ctx, command, args...)
	return err
}

func (a linuxAdapter) runNVM(ctx context.Context, args ...string) error {
	nvmExec := filepath.Join(a.config.Home, ".nvm", "nvm-exec")
	return a.userRun(ctx, nvmExec, append([]string{"nvm"}, args...)...)
}

func (a linuxAdapter) userRun(ctx context.Context, command string, args ...string) error {
	_, err := a.runner.Run(ctx, "env", append([]string{"HOME=" + a.config.Home, command}, args...)...)
	return err
}

func (a linuxAdapter) ensureProfileBlock(name, content string) error {
	profilePath := filepath.Join(a.config.Home, ".bashrc")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0700); err != nil {
		return err
	}
	existing, err := os.ReadFile(profilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	start := "# >>> johanbostrom jb: " + name + " >>>"
	end := "# <<< johanbostrom jb: " + name + " <<<"
	lines := strings.Split(string(existing), "\n")
	kept := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		if line == start {
			skipping = true
			continue
		}
		if line == end {
			skipping = false
			continue
		}
		if !skipping {
			kept = append(kept, line)
		}
	}
	base := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if base != "" {
		base += "\n\n"
	}
	updated := base + start + "\n" + content + "\n" + end + "\n"
	temporary := profilePath + ".jb.tmp"
	if err := os.WriteFile(temporary, []byte(updated), 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, profilePath); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
