// Package adapters provides platform-specific tool operations.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
	"github.com/zarxor/cli/internal/tools"
)

// Adapter is the platform boundary used by installation planning.
type Adapter interface {
	Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error)
	Install(ctx context.Context, tool tools.Tool) error
	Update(ctx context.Context, tool tools.Tool) error
	Verify(ctx context.Context, tool tools.Tool) error
}

// LinuxConfig supplies host facts which platform detection already knows and
// keeps adapter fixtures independent from the machine running the tests. Root
// defaults to false; Home, TempDir, and Architecture receive local defaults.
// Callers must provide the invoking user's Home and the live Distribution and
// Codename values before configuring Docker on Debian or Ubuntu.
type LinuxConfig struct {
	Root         bool
	Home         string
	TempDir      string
	InvokingUser string
	InvokingUID  int
	InvokingGID  int
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
	githubCLIKeyURL     = "https://cli.github.com/packages/githubcli-archive-keyring.gpg"
	nvmPinnedCommit     = "d025499c7f5466d0dc0a324dc98eab72cce8377d" // nvm v0.40.3
	profileUpdateHelper = `#!/bin/sh
set -eu

profile=$1
name=$2
content=$3
directory=${profile%/*}
[ "$directory" != "$profile" ] || directory=.
mkdir -p -- "$directory"

start="# >>> johanbostrom jb: $name >>>"
end="# <<< johanbostrom jb: $name <<<"
mode=600
if [ -e "$profile" ]; then
	mode=$(stat -c %a -- "$profile")
fi

temporary=$(mktemp "$directory/.jb-profile-XXXXXX")
cleanup() {
	if [ -n "${temporary:-}" ]; then
		rm -f -- "$temporary"
	fi
}
trap cleanup EXIT HUP INT TERM

if [ -e "$profile" ]; then
	awk -v start="$start" -v end="$end" '
		$0 == start { skipping = 1; next }
		$0 == end { skipping = 0; next }
		!skipping { kept[++count] = $0 }
		END {
			while (count > 0 && kept[count] == "") count--
			for (index = 1; index <= count; index++) print kept[index]
			if (count > 0) print ""
		}
	' "$profile" > "$temporary"
fi
printf '%s\n%s\n%s\n' "$start" "$content" "$end" >> "$temporary"
chmod "$mode" "$temporary"
mv -f -- "$temporary" "$profile"
temporary=
`
)

type linuxAdapter struct {
	runner                 runner.Runner
	elevation              runner.Elevation
	config                 LinuxConfig
	aptUpdated             bool
	githubCLIConfigured    bool
	dockerRepositoryConfig bool
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
	if elevation == nil {
		elevation = sudoElevation{runner: commandRunner}
	}
	return linuxAdapter{runner: commandRunner, elevation: elevation, config: config}
}

type sudoElevation struct{ runner runner.Runner }

func (s sudoElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	return s.runner.Run(ctx, "sudo", append([]string{command}, args...)...)
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
	if err != nil {
		return detection, err
	}
	packages, ok := debianPackages(tool.ID)
	if !ok || len(packages) == 0 {
		return detection, nil
	}
	if detection.Installed && a.debianPackageOwnership(ctx, tool) == ownershipNotOwned {
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
		if err := a.updateAptMetadata(ctx); err != nil {
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
	return a.installUserTool(ctx, tool)
}

func (a *DebianAdapter) Update(ctx context.Context, tool tools.Tool) error {
	if packages, ok := debianPackages(tool.ID); ok {
		if a.debianPackageOwnership(ctx, tool) == ownershipNotOwned {
			return fmt.Errorf("%s is not owned by apt; refusing to update a different installation", tool.Name)
		}
		if tool.ID == profile.GitHubCLI {
			if err := a.configureGitHubCLI(ctx); err != nil {
				return err
			}
		}
		if isDockerTool(tool.ID) {
			if err := a.configureDocker(ctx); err != nil {
				return err
			}
			if tool.ID == profile.Docker {
				if err := a.removeDockerConflicts(ctx); err != nil {
					return err
				}
				if err := a.updateAptMetadata(ctx); err != nil {
					return err
				}
				if err := a.system(ctx, "apt-get", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"); err != nil {
					return err
				}
				return a.system(ctx, "systemctl", "enable", "--now", "docker.service")
			}
		}
		if err := a.updateAptMetadata(ctx); err != nil {
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

func (a *DebianAdapter) debianPackageOwnership(ctx context.Context, tool tools.Tool) ownershipStatus {
	command, _, err := a.versionCommand(tool.ID)
	if err != nil {
		return ownershipUnknown
	}
	executable, err := a.runner.LookPath(ctx, command)
	if err != nil {
		// The normal planning path already detected the executable. Keep direct
		// adapter calls compatible when they intentionally skip discovery.
		return ownershipUnknown
	}
	result, err := a.runner.Run(ctx, "dpkg-query", "-S", executable)
	if err != nil {
		return ownershipNotOwned
	}
	output := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	if output == "" {
		return ownershipUnknown
	}
	owner := packageOwnerFromDpkg(output)
	if !packageNameMatches(owner, debianExecutablePackages(tool.ID)) {
		return ownershipNotOwned
	}
	if tool.ID == profile.DockerBuildx || tool.ID == profile.DockerCompose {
		componentPackages, _ := debianPackages(tool.ID)
		component := componentPackages[0]
		componentStatus := a.debianPackageInstalled(ctx, component)
		if componentStatus == ownershipNotOwned {
			return ownershipNotOwned
		}
	}
	return ownershipOwned
}

func debianExecutablePackages(id tools.ToolID) []string {
	switch id {
	case profile.Docker, profile.DockerBuildx, profile.DockerCompose:
		return []string{"docker-ce-cli"}
	default:
		packages, _ := debianPackages(id)
		return packages
	}
}

func (a *DebianAdapter) debianPackageInstalled(ctx context.Context, packageName string) ownershipStatus {
	result, err := a.runner.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}", packageName)
	if err != nil {
		return ownershipNotOwned
	}
	output := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	if output == "" {
		return ownershipUnknown
	}
	if strings.EqualFold(output, "installed") {
		return ownershipOwned
	}
	return ownershipNotOwned
}

func packageOwnerFromDpkg(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if owner, _, ok := strings.Cut(line, ":"); ok {
			return strings.TrimSpace(owner)
		}
	}
	return ""
}

func packageNameMatches(owner string, expected []string) bool {
	owner = strings.TrimSpace(strings.SplitN(owner, ":", 2)[0])
	for _, packageName := range expected {
		if owner == packageName {
			return true
		}
	}
	return false
}

func isDockerTool(id tools.ToolID) bool {
	return id == profile.Docker || id == profile.DockerBuildx || id == profile.DockerCompose
}

func (a *DebianAdapter) configureGitHubCLI(ctx context.Context) error {
	if a.githubCLIConfigured {
		return nil
	}
	if err := a.system(ctx, "install", "-m", "0755", "-d", "/etc/apt/keyrings", "/etc/apt/sources.list.d"); err != nil {
		return err
	}
	key, err := a.downloadTemporary(ctx, "jb-githubcli-key-*", githubCLIKeyURL)
	if err != nil {
		return err
	}
	defer os.Remove(key)
	if err := a.system(ctx, "install", "-m", "0644", key, "/etc/apt/keyrings/githubcli-archive-keyring.gpg"); err != nil {
		return err
	}
	content := fmt.Sprintf("deb [arch=%s signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n", a.config.Architecture)
	source, err := createTemporary(a.config.TempDir, "jb-github-cli-source-*", content)
	if err != nil {
		return err
	}
	defer os.Remove(source)
	if err := a.system(ctx, "install", "-m", "0644", source, "/etc/apt/sources.list.d/github-cli.list"); err != nil {
		return err
	}
	a.githubCLIConfigured = true
	a.aptUpdated = false
	return nil
}

func (a *DebianAdapter) configureDocker(ctx context.Context) error {
	if a.dockerRepositoryConfig {
		return nil
	}
	if a.config.Codename == "" {
		return fmt.Errorf("Docker apt repository requires a distribution codename")
	}
	if a.config.Distribution != "debian" && a.config.Distribution != "ubuntu" {
		return fmt.Errorf("Docker apt repository does not support distribution %q", a.config.Distribution)
	}
	if err := a.system(ctx, "install", "-m", "0755", "-d", "/etc/apt/keyrings", "/etc/apt/sources.list.d"); err != nil {
		return err
	}
	keyURL := "https://download.docker.com/linux/" + a.config.Distribution + "/gpg"
	key, err := a.downloadTemporary(ctx, "jb-docker-key-*", keyURL)
	if err != nil {
		return err
	}
	defer os.Remove(key)
	if err := a.system(ctx, "install", "-m", "0644", key, "/etc/apt/keyrings/docker.asc"); err != nil {
		return err
	}
	content := fmt.Sprintf("Types: deb\nURIs: https://download.docker.com/linux/%s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n", a.config.Distribution, a.config.Codename, a.config.Architecture)
	source, err := createTemporary(a.config.TempDir, "jb-docker-source-*", content)
	if err != nil {
		return err
	}
	defer os.Remove(source)
	if err := a.system(ctx, "install", "-m", "0644", source, "/etc/apt/sources.list.d/docker.sources"); err != nil {
		return err
	}
	a.dockerRepositoryConfig = true
	a.aptUpdated = false
	return nil
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
	_, err := a.elevation.RunElevated(ctx, command, args...)
	return err
}

func (a *DebianAdapter) updateAptMetadata(ctx context.Context) error {
	if a.aptUpdated {
		return nil
	}
	if err := a.system(ctx, "apt-get", "update"); err != nil {
		return err
	}
	a.aptUpdated = true
	return nil
}

func (a linuxAdapter) downloadTemporary(ctx context.Context, pattern, url string) (string, error) {
	destination, err := createTemporary(a.config.TempDir, pattern, "")
	if err != nil {
		return "", err
	}
	if _, err := a.runner.Run(ctx, "curl", "-fsSL", url, "-o", destination); err != nil {
		_ = os.Remove(destination)
		return "", err
	}
	return destination, nil
}

func createTemporary(directory, pattern, content string) (string, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if _, err := file.WriteString(content); err != nil {
		cleanup()
		return "", err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", err
	}
	return path, nil
}

func (a linuxAdapter) detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	detection, err := a.detectCurrent(ctx, tool)
	if err != nil || !detection.Installed {
		return detection, err
	}
	candidate, supported, err := a.userToolCandidate(ctx, tool.ID)
	if err != nil {
		return detection, nil
	}
	if supported {
		detection.Candidate = candidate
	}
	return detection, nil
}

func (a linuxAdapter) detectCurrent(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	if tool.ID == profile.NVM {
		return a.detectNVM(ctx)
	}
	if executable, ok := nvmExecutable(tool.ID); ok {
		return a.detectNVMExecutable(ctx, executable)
	}
	command, args, err := a.versionCommand(tool.ID)
	if err != nil {
		return detect.Detection{}, err
	}
	if _, err := a.runner.LookPath(ctx, command); err != nil {
		return detect.Detection{Installed: false}, nil
	}
	result, err := a.runner.Run(ctx, command, args...)
	if err != nil {
		if expectedMissingComponent(tool.ID, result) {
			return detect.Detection{Installed: false}, nil
		}
		return detect.Detection{}, err
	}
	return detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}, nil
}

func (a linuxAdapter) userToolCandidate(ctx context.Context, id tools.ToolID) (string, bool, error) {
	switch id {
	case profile.NVM:
		version, err := a.latestNVMVersion(ctx)
		return version, true, err
	case profile.Node:
		result, err := a.runner.Run(ctx, "curl", "-fsSL", "https://nodejs.org/dist/index.json")
		if err != nil {
			return "", true, err
		}
		var releases []struct {
			Version string          `json:"version"`
			LTS     json.RawMessage `json:"lts"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &releases); err != nil {
			return "", true, fmt.Errorf("parse Node.js release metadata: %w", err)
		}
		for _, release := range releases {
			var ltsName string
			if json.Unmarshal(release.LTS, &ltsName) == nil && ltsName != "" {
				return release.Version, true, nil
			}
		}
		return "", true, fmt.Errorf("Node.js release metadata contains no LTS version")
	case profile.NPM, profile.Corepack, profile.PNPM, profile.Yarn, profile.Codex, profile.Bun:
		packageName := map[tools.ToolID]string{
			profile.NPM: "npm", profile.Corepack: "corepack", profile.PNPM: "pnpm",
			profile.Yarn: "@yarnpkg/cli-dist", profile.Codex: "@openai/codex", profile.Bun: "bun",
		}[id]
		result, err := a.runNVMExecutableCommand(ctx, "npm", "view", packageName, "version")
		if err != nil {
			return "", true, err
		}
		return detect.ParseVersion(result.Stdout, result.Stderr), true, nil
	default:
		return "", false, nil
	}
}

func (a linuxAdapter) verify(ctx context.Context, tool tools.Tool) error {
	detection, err := a.detectCurrent(ctx, tool)
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
		return filepath.Join(a.config.Home, ".local", "bin", "codex"), []string{"--version"}, nil
	case profile.Bun:
		return filepath.Join(a.config.Home, ".bun", "bin", "bun"), []string{"--version"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported tool %q", id)
	}
}

func (a linuxAdapter) installUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Codex:
		return a.runNVMExecutable(ctx, "npm", "install", "--global", "@openai/codex@latest")
	case profile.NVM:
		return a.installNVM(ctx, "v0.40.3")
	case profile.Node:
		return a.convergeNodeLTS(ctx)
	case profile.NPM:
		return a.runNVMExecutable(ctx, "npm", "install", "--global", "npm@latest")
	case profile.Corepack:
		if err := a.runNVMExecutable(ctx, "npm", "install", "--global", "corepack@latest"); err != nil {
			return err
		}
		return a.runNVMExecutable(ctx, "corepack", "enable")
	case profile.PNPM:
		return a.runNVMExecutable(ctx, "corepack", "prepare", "pnpm@latest", "--activate")
	case profile.Yarn:
		return a.runNVMExecutable(ctx, "corepack", "prepare", "yarn@stable", "--activate")
	case profile.Bun:
		return a.runNVMExecutable(ctx, "npm", "install", "--global", "bun@latest")
	default:
		return fmt.Errorf("unsupported tool %q", tool.ID)
	}
}

func (a linuxAdapter) updateUserTool(ctx context.Context, tool tools.Tool) error {
	switch tool.ID {
	case profile.Bun:
		return a.runNVMExecutable(ctx, "npm", "install", "--global", "bun@latest")
	case profile.NVM:
		version, err := a.latestNVMVersion(ctx)
		if err != nil {
			return err
		}
		return a.updateNVM(ctx, version)
	case profile.Node:
		return a.convergeNodeLTS(ctx)
	default:
		return a.installUserTool(ctx, tool)
	}
}

func (a linuxAdapter) installNVM(ctx context.Context, version string) error {
	nvmDir := filepath.Join(a.config.Home, ".nvm")
	if err := a.userRun(ctx, "git", "clone", "--branch", version, "--depth", "1", "https://github.com/nvm-sh/nvm.git", nvmDir); err != nil {
		return err
	}
	if err := a.userRun(ctx, "git", "-C", nvmDir, "checkout", "--detach", nvmPinnedCommit); err != nil {
		return err
	}
	return a.ensureProfileBlock(ctx, "nvm", "export NVM_DIR=\"$HOME/.nvm\"\n[ -s \"$NVM_DIR/nvm.sh\" ] && \\. \"$NVM_DIR/nvm.sh\"\n[ -s \"$NVM_DIR/bash_completion\" ] && \\. \"$NVM_DIR/bash_completion\"")
}

func (a linuxAdapter) updateNVM(ctx context.Context, version string) error {
	nvmDir := filepath.Join(a.config.Home, ".nvm")
	if err := a.userRun(ctx, "git", "-C", nvmDir, "fetch", "--depth", "1", "origin", "tag", version); err != nil {
		return err
	}
	if err := a.userRun(ctx, "git", "-C", nvmDir, "checkout", "--detach", version); err != nil {
		return err
	}
	return a.ensureProfileBlock(ctx, "nvm", "export NVM_DIR=\"$HOME/.nvm\"\n[ -s \"$NVM_DIR/nvm.sh\" ] && \\. \"$NVM_DIR/nvm.sh\"\n[ -s \"$NVM_DIR/bash_completion\" ] && \\. \"$NVM_DIR/bash_completion\"")
}

func (a linuxAdapter) latestNVMVersion(ctx context.Context) (string, error) {
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
	return latest, nil
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

func (a linuxAdapter) runNVM(ctx context.Context, args ...string) error {
	_, err := a.runNVMCommand(ctx, args...)
	return err
}

func (a linuxAdapter) convergeNodeLTS(ctx context.Context) error {
	if err := a.runNVM(ctx, "install", "--lts", "--latest-npm"); err != nil {
		return err
	}
	return a.runNVM(ctx, "alias", "default", "lts/*")
}

func (a linuxAdapter) runNVMCommand(ctx context.Context, args ...string) (runner.Result, error) {
	helper, err := createTemporary(a.config.TempDir, "jb-nvm-command-*", "#!/bin/sh\nset -eu\n. \"$NVM_DIR/nvm.sh\"\nnvm \"$@\"\n")
	if err != nil {
		return runner.Result{}, err
	}
	defer os.Remove(helper)
	if err := os.Chmod(helper, 0644); err != nil {
		return runner.Result{}, err
	}
	return a.runUserCommand(ctx, []string{"NVM_DIR=" + filepath.Join(a.config.Home, ".nvm")}, "sh", append([]string{helper}, args...)...)
}

func (a linuxAdapter) runNVMExecutable(ctx context.Context, executable string, args ...string) error {
	_, err := a.runNVMExecutableCommand(ctx, executable, args...)
	return err
}

func (a linuxAdapter) runNVMExecutableCommand(ctx context.Context, executable string, args ...string) (runner.Result, error) {
	nvmExec := filepath.Join(a.config.Home, ".nvm", "nvm-exec")
	return a.runUserCommand(ctx, []string{"NVM_DIR=" + filepath.Join(a.config.Home, ".nvm"), "NODE_VERSION=lts/*"}, nvmExec, append([]string{executable}, args...)...)
}

func (a linuxAdapter) detectNVM(ctx context.Context) (detect.Detection, error) {
	nvmExec := filepath.Join(a.config.Home, ".nvm", "nvm-exec")
	if _, err := a.runner.LookPath(ctx, nvmExec); err != nil {
		return detect.Detection{Installed: false}, nil
	}
	result, err := a.runNVMCommand(ctx, "--version")
	if err != nil {
		return detect.Detection{}, err
	}
	return detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}, nil
}

func (a linuxAdapter) detectNVMExecutable(ctx context.Context, executable string) (detect.Detection, error) {
	nvmExec := filepath.Join(a.config.Home, ".nvm", "nvm-exec")
	if _, err := a.runner.LookPath(ctx, nvmExec); err != nil {
		return detect.Detection{Installed: false}, nil
	}
	result, err := a.runUserCommand(ctx, []string{"NVM_DIR=" + filepath.Join(a.config.Home, ".nvm"), "NODE_VERSION=lts/*"}, nvmExec, executable, "--version")
	if err != nil {
		if expectedMissingComponentExecutable(executable, result) {
			return detect.Detection{Installed: false}, nil
		}
		return detect.Detection{}, err
	}
	return detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}, nil
}

func expectedMissingComponent(id tools.ToolID, result runner.Result) bool {
	if result.ExitCode != 1 {
		return false
	}
	component := ""
	switch id {
	case profile.DockerBuildx:
		component = "buildx"
	case profile.DockerCompose:
		component = "compose"
	default:
		return false
	}
	for _, line := range strings.Split(strings.ToLower(result.Stdout+"\n"+result.Stderr), "\n") {
		line = strings.TrimSpace(line)
		if line == "docker: '"+component+"' is not a docker command" ||
			line == "docker: \""+component+"\" is not a docker command" {
			return true
		}
	}
	return false
}

func expectedMissingComponentExecutable(executable string, result runner.Result) bool {
	if result.ExitCode != 127 {
		return false
	}
	switch executable {
	case "node", "npm", "corepack", "pnpm", "yarn", "codex", "bun":
		// nvm-exec exits 127 when the requested version (normally lts/*) has
		// not been installed yet. Depending on the nvm version, the diagnostic
		// is either suppressed or written as an N/A/not-yet-installed message.
		if expectedMissingNVMVersion(result) {
			return true
		}
		for _, line := range strings.Split(strings.ToLower(result.Stdout+"\n"+result.Stderr), "\n") {
			if expectedNVMExecMissingLine(strings.TrimSpace(line), executable) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func expectedMissingNVMVersion(result runner.Result) bool {
	if result.ExitCode != 127 {
		return false
	}
	output := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	return output == "" ||
		(strings.Contains(output, "n/a: version") && strings.Contains(output, "not yet installed"))
}

func expectedNVMExecMissingLine(line, executable string) bool {
	for _, diagnostic := range []string{executable + ": not found", executable + ": command not found"} {
		if line == diagnostic {
			return true
		}
		scriptAndLine, ok := strings.CutSuffix(line, ": exec: "+diagnostic)
		if !ok {
			continue
		}
		separator := strings.LastIndex(scriptAndLine, ": ")
		if separator < 0 || filepath.Base(scriptAndLine[:separator]) != "nvm-exec" {
			continue
		}
		lineNumber := strings.TrimPrefix(scriptAndLine[separator+2:], "line ")
		if _, err := strconv.Atoi(lineNumber); err == nil {
			return true
		}
	}
	return false
}

func nvmExecutable(id tools.ToolID) (string, bool) {
	switch id {
	case profile.Node:
		return "node", true
	case profile.NPM:
		return "npm", true
	case profile.Corepack:
		return "corepack", true
	case profile.PNPM:
		return "pnpm", true
	case profile.Yarn:
		return "yarn", true
	case profile.Codex:
		return "codex", true
	case profile.Bun:
		return "bun", true
	default:
		return "", false
	}
}

func (a linuxAdapter) userRun(ctx context.Context, command string, args ...string) error {
	_, err := a.runUserCommand(ctx, nil, command, args...)
	return err
}

func (a linuxAdapter) runUserCommand(ctx context.Context, environment []string, command string, args ...string) (runner.Result, error) {
	commandArgs := append([]string{"HOME=" + a.config.Home}, environment...)
	commandArgs = append(commandArgs, command)
	commandArgs = append(commandArgs, args...)
	if a.config.Root && a.config.InvokingUser != "" && a.config.InvokingUser != "root" {
		return a.runner.Run(ctx, "sudo", append([]string{"-H", "-u", a.config.InvokingUser, "env"}, commandArgs...)...)
	}
	return a.runner.Run(ctx, "env", commandArgs...)
}

func (a linuxAdapter) ensureProfileBlock(ctx context.Context, name, content string) error {
	profilePath := filepath.Join(a.config.Home, ".bashrc")
	if a.config.Root && a.config.InvokingUser != "" && a.config.InvokingUser != "root" {
		return a.ensureProfileBlockAsUser(ctx, profilePath, name, content)
	}
	if err := os.MkdirAll(filepath.Dir(profilePath), 0700); err != nil {
		return err
	}
	existing, err := os.ReadFile(profilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := updatedProfile(string(existing), name, content)
	temporary, err := createTemporary(filepath.Dir(profilePath), ".jb-profile-*", updated)
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, profilePath); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (a linuxAdapter) ensureProfileBlockAsUser(ctx context.Context, profilePath, name, content string) error {
	helper, err := createTemporary(a.config.TempDir, "jb-profile-update-*", profileUpdateHelper)
	if err != nil {
		return err
	}
	defer os.Remove(helper)
	if err := os.Chmod(helper, 0644); err != nil {
		return err
	}
	_, err = a.runUserCommand(ctx, nil, "sh", helper, profilePath, name, content)
	return err
}

func updatedProfile(existing, name, content string) string {
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
	return base + start + "\n" + content + "\n" + end + "\n"
}
