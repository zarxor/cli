package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

func TestDebianNonRootPrefixesOnlySystemOperations(t *testing.T) {
	fixture := runner.NewFixture()
	elevation := &fixtureElevation{fixture: fixture}
	adapter := NewDebianAdapter(fixture, elevation, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: t.TempDir(),
	})
	adapter.(*DebianAdapter).config.Root = false

	if err := adapter.Install(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Install(context.Background(), mustTool(t, profile.Codex)); err != nil {
		t.Fatal(err)
	}

	wantFirst := runner.Command{Command: "sudo", Args: []string{"apt-get", "update"}}
	if got := fixture.Commands[0]; !reflect.DeepEqual(got, wantFirst) {
		t.Fatalf("first command = %#v, want %#v", got, wantFirst)
	}
	if elevation.calls == 0 {
		t.Fatal("system operations did not use runner.Elevation")
	}
	for _, command := range fixture.Commands {
		if command.Command == "sudo" && slicesContain(command.Args, "sh") {
			t.Fatalf("user installer was elevated: %#v", command)
		}
	}
}

func TestRootRunsDebianSystemOperationsDirectly(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: t.TempDir(),
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}

	want := []runner.Command{
		{Command: "apt-get", Args: []string{"update"}},
		{Command: "apt-get", Args: []string{"install", "-y", "git"}},
	}
	if !reflect.DeepEqual(fixture.Commands, want) {
		t.Fatalf("commands = %#v, want %#v", fixture.Commands, want)
	}
}

func TestDebianGitHubCLIConvergesSupportedRepository(t *testing.T) {
	fixture := runner.NewFixture()
	capture := &fileCapturingRunner{fixture: fixture, files: make(map[string]string)}
	temp := t.TempDir()
	predictableSource := filepath.Join(temp, "github-cli.list")
	if err := os.WriteFile(predictableSource, []byte("pre-existing marker"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewDebianAdapter(capture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: temp, Architecture: "amd64",
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.GitHubCLI)); err != nil {
		t.Fatal(err)
	}

	wantSource := "deb [arch=amd64 signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n"
	if gotSource := capture.files["/etc/apt/sources.list.d/github-cli.list"]; gotSource != wantSource {
		t.Fatalf("repository definition = %q, want %q", gotSource, wantSource)
	}
	key := curlDestination(t, fixture.Commands, githubCLIKeyURL)
	assertInstalledFrom(t, fixture.Commands, key, "/etc/apt/keyrings/githubcli-archive-keyring.gpg")
	assertTemporaryRemoved(t, key)
	source := installedSource(t, fixture.Commands, "/etc/apt/sources.list.d/github-cli.list")
	if !strings.HasPrefix(filepath.Base(source), "jb-github-cli-source-") {
		t.Fatalf("repository source path = %q, want unique path", source)
	}
	assertTemporaryRemoved(t, source)
	marker, err := os.ReadFile(predictableSource)
	if err != nil || string(marker) != "pre-existing marker" {
		t.Fatalf("pre-existing repository file changed: content %q, error %v", marker, err)
	}
}

func TestDebianDockerRemovesOnlyInstalledConflictCandidates(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "docker.io"}, runner.Result{Stdout: "installed\n"}, nil)
	fixture.Set("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "containerd"}, runner.Result{Stdout: "installed\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: t.TempDir(), Distribution: "debian",
		Codename: "bookworm", Architecture: "amd64",
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Docker)); err != nil {
		t.Fatal(err)
	}

	for _, candidate := range DockerConflictCandidates {
		assertHasCommand(t, fixture.Commands, "dpkg-query", "-W", "-f=${db:Status-Status}", candidate)
	}
	assertHasCommand(t, fixture.Commands, "apt-get", "remove", "-y", "docker.io", "containerd")
	assertHasCommand(t, fixture.Commands, "apt-get", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io")
	for _, command := range fixture.Commands {
		if slicesContain(command.Args, "/var/lib/docker") {
			t.Fatalf("Docker data directory was mutated: %#v", command)
		}
	}
}

func TestDebianUpdateUsesAptUpgradePlan(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}

	want := []runner.Command{
		{Command: "apt-get", Args: []string{"update"}},
		{Command: "apt-get", Args: []string{"install", "--only-upgrade", "-y", "git"}},
	}
	if !reflect.DeepEqual(fixture.Commands, want) {
		t.Fatalf("commands = %#v, want %#v", fixture.Commands, want)
	}
}

func TestDebianUpdateConvergesDockerRepository(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: temp, Distribution: "ubuntu",
		Codename: "noble", Architecture: "amd64",
	})

	if err := adapter.Update(context.Background(), mustTool(t, profile.DockerBuildx)); err != nil {
		t.Fatal(err)
	}

	key := curlDestination(t, fixture.Commands, "https://download.docker.com/linux/ubuntu/gpg")
	assertInstalledFrom(t, fixture.Commands, key, "/etc/apt/keyrings/docker.asc")
	assertInstallDestination(t, fixture.Commands, "/etc/apt/sources.list.d/docker.sources")
	assertTemporaryRemoved(t, key)
}

func TestArchInstallAndUpdateUseNeededPacmanTransactions(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewArchAdapter(fixture, &fixtureElevation{fixture: fixture}, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})
	ctx := context.Background()

	if err := adapter.Install(ctx, mustTool(t, profile.GitHubCLI)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Update(ctx, mustTool(t, profile.GitHubCLI)); err != nil {
		t.Fatal(err)
	}

	want := runner.Command{Command: "sudo", Args: []string{"pacman", "-Syu", "--noconfirm", "--needed", "github-cli"}}
	if len(fixture.Commands) != 2 || !reflect.DeepEqual(fixture.Commands[0], want) || !reflect.DeepEqual(fixture.Commands[1], want) {
		t.Fatalf("commands = %#v, want two %#v transactions", fixture.Commands, want)
	}
}

func TestArchBunInstallsUnzipWithoutElevatingInstaller(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewArchAdapter(fixture, &fixtureElevation{fixture: fixture}, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "sudo", "pacman", "-Syu", "--noconfirm", "--needed", "unzip")
	installer := curlDestination(t, fixture.Commands, bunInstaller)
	if !strings.HasPrefix(filepath.Base(installer), "jb-bun-install-") {
		t.Fatalf("Bun installer path = %q", installer)
	}
	assertTemporaryRemoved(t, installer)
	for _, command := range fixture.Commands {
		if command.Command == "sudo" && slicesContain(command.Args, "jb-bun-install.sh") {
			t.Fatalf("Bun installer was elevated: %#v", command)
		}
	}
}

func TestDebianBunInstallsUnzipBeforeUserInstaller(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewDebianAdapter(fixture, &fixtureElevation{fixture: fixture}, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}

	want := runner.Command{Command: "sudo", Args: []string{"apt-get", "install", "-y", "unzip"}}
	if len(fixture.Commands) == 0 || !reflect.DeepEqual(fixture.Commands[0], want) {
		t.Fatalf("first command = %#v, want %#v", fixture.Commands[0], want)
	}
}

func TestLinuxProfileBlocksAreIdempotent(t *testing.T) {
	home := t.TempDir()
	fixture := runner.NewFixture()
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})
	tool := mustTool(t, profile.NVM)

	if err := adapter.Install(context.Background(), tool); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Install(context.Background(), tool); err != nil {
		t.Fatal(err)
	}

	profileBytes, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	profileText := string(profileBytes)
	if got := strings.Count(profileText, "# >>> johanbostrom jb: nvm >>>"); got != 1 {
		t.Fatalf("nvm profile block count = %d, want 1\n%s", got, profileText)
	}
}

func TestDebianDetectsInstalledToolVersion(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "/usr/bin/git"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.47.1\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.Current != "git version 2.47.1" {
		t.Fatalf("Detect() = %#v", got)
	}
}

func TestDebianDetectsCandidatePackageVersion(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "/usr/bin/git"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.47.1\n"}, nil)
	fixture.Set("apt-cache", []string{"policy", "git"}, runner.Result{Stdout: "Installed: 1:2.47.1-0\nCandidate: 1:2.48.0-1\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != "1:2.48.0-1" {
		t.Fatalf("candidate = %q, want 1:2.48.0-1", got.Candidate)
	}
}

func TestArchDetectsCandidatePackageVersion(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "/usr/bin/git"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.47.1\n"}, nil)
	fixture.Set("pacman", []string{"-Si", "git"}, runner.Result{Stdout: "Repository      : core\nName            : git\nVersion         : 2.48.0-1\n"}, nil)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != "2.48.0-1" {
		t.Fatalf("candidate = %q, want 2.48.0-1", got.Candidate)
	}
}

func TestDebianCandidateIsDetectedWhenPackageIsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("apt-cache", []string{"policy", "git"}, runner.Result{Stdout: "Installed: (none)\nCandidate: 1:2.48.0-1\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got.Installed || got.Candidate != "1:2.48.0-1" {
		t.Fatalf("Detect() = %#v, want absent with candidate 1:2.48.0-1", got)
	}
}

func TestArchCandidateIsDetectedWhenPackageIsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("pacman", []string{"-Si", "git"}, runner.Result{Stdout: "Name            : git\nVersion         : 2.48.0-1\n"}, nil)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got.Installed || got.Candidate != "2.48.0-1" {
		t.Fatalf("Detect() = %#v, want absent with candidate 2.48.0-1", got)
	}
}

func TestDebianDockerUpdateMigratesConflictsToCompleteOfficialPackages(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("dpkg-query", []string{"-W", "-f=${db:Status-Status}", "docker.io"}, runner.Result{Stdout: "installed\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: t.TempDir(), Distribution: "debian",
		Codename: "bookworm", Architecture: "amd64",
	})

	if err := adapter.Update(context.Background(), mustTool(t, profile.Docker)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "apt-get", "remove", "-y", "docker.io")
	assertHasCommand(t, fixture.Commands, "apt-get", "install", "-y", "docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin")
	for _, command := range fixture.Commands {
		if command.Command == "apt-get" && slicesContain(command.Args, "--only-upgrade") {
			t.Fatalf("Docker migration used upgrade-only install: %#v", command)
		}
	}
}

func TestInstallerUsesUniqueTemporaryFileWithoutTouchingPreexistingName(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	home := t.TempDir()
	predictable := filepath.Join(temp, "jb-codex-install.sh")
	if err := os.WriteFile(predictable, []byte("attacker-owned marker"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Codex)); err != nil {
		t.Fatal(err)
	}

	marker, err := os.ReadFile(predictable)
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != "attacker-owned marker" {
		t.Fatalf("pre-existing predictable file was changed to %q", marker)
	}
	destination := curlDestination(t, fixture.Commands, codexInstaller)
	if destination == predictable || !strings.HasPrefix(filepath.Base(destination), "jb-codex-install-") {
		t.Fatalf("installer destination = %q, want unique jb-codex-install-* path", destination)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unique installer cleanup error = %v", err)
	}
}

func TestNodeInstallUsesArgvSafeNVMHelperInsteadOfNVMExecFunctionCall(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	home := t.TempDir()
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Node)); err != nil {
		t.Fatal(err)
	}

	var helper string
	for _, command := range fixture.Commands {
		if command.Command == "env" && slicesContain(command.Args, "sh") && slicesContain(command.Args, "install") {
			for index, arg := range command.Args {
				if arg == "sh" && index+1 < len(command.Args) {
					helper = command.Args[index+1]
				}
			}
		}
		if slicesContain(command.Args, "nvm-exec") && slicesContain(command.Args, "nvm") {
			t.Fatalf("nvm shell function was passed to nvm-exec: %#v", command)
		}
	}
	if helper == "" {
		t.Fatalf("commands = %#v, want argv-safe nvm helper", fixture.Commands)
	}
	if _, err := os.Stat(helper); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nvm helper cleanup error = %v", err)
	}
}

func TestCleanProcessDetectsCodexAndNodeToolsFromConfiguredHome(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	temp := t.TempDir()
	codex := filepath.Join(home, ".local", "bin", "codex")
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture.LookPaths[codex] = codex
	fixture.LookPaths[nvmExec] = nvmExec
	fixture.Set(codex, []string{"--version"}, runner.Result{Stdout: "codex-cli 1.2.3\n"}, nil)
	fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), nvmExec, "node", "--version"}, runner.Result{Stdout: "v24.1.0\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	for _, test := range []struct {
		id      profile.ToolID
		version string
	}{{profile.Codex, "codex-cli 1.2.3"}, {profile.Node, "v24.1.0"}} {
		got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Installed || got.Current != test.version {
			t.Fatalf("Detect(%s) = %#v, want %q from configured home", test.id, got, test.version)
		}
	}
}

func TestInstallerFailurePreservesExitStatusAndCleansTemporaryFile(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	wantErr := &fixtureExitError{status: 23}
	home := t.TempDir()
	failing := &installerFailureRunner{fixture: fixture, err: wantErr, status: 23, namePrefix: "jb-codex-install-"}
	adapter := NewDebianAdapter(failing, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	err := adapter.Install(context.Background(), mustTool(t, profile.Codex))
	if !errors.Is(err, wantErr) || err.(*fixtureExitError).ExitCode() != 23 {
		t.Fatalf("Install() error = %#v, want unchanged exit status 23", err)
	}
	installer := curlDestination(t, fixture.Commands, codexInstaller)
	assertTemporaryRemoved(t, installer)
}

func TestInstallerNVMUpdateResolvesLatestStableReleaseAndCleansTemporaryFile(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	home := t.TempDir()
	fixture.Set("git", []string{"ls-remote", "--tags", "--refs", "https://github.com/nvm-sh/nvm.git", "v*"}, runner.Result{Stdout: "aaa\trefs/tags/v0.39.7\nbbb\trefs/tags/v0.40.3\nccc\trefs/tags/v0.40.10\n"}, nil)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	if err := adapter.Update(context.Background(), mustTool(t, profile.NVM)); err != nil {
		t.Fatal(err)
	}

	installer := curlDestination(t, fixture.Commands, "https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.10/install.sh")
	assertTemporaryRemoved(t, installer)
}

type fixtureExitError struct{ status int }

func (e *fixtureExitError) Error() string { return "fixture installer failed" }
func (e *fixtureExitError) ExitCode() int { return e.status }

func mustTool(t *testing.T, id profile.ToolID) tools.Tool {
	t.Helper()
	tool, ok := tools.Lookup(id)
	if !ok {
		t.Fatalf("tool %q missing from catalog", id)
	}
	return tool
}

func assertHasCommand(t *testing.T, commands []runner.Command, command string, args ...string) {
	t.Helper()
	want := runner.Command{Command: command, Args: args}
	for _, got := range commands {
		if reflect.DeepEqual(got, want) {
			return
		}
	}
	t.Fatalf("commands %#v do not contain %#v", commands, want)
}

func curlDestination(t *testing.T, commands []runner.Command, url string) string {
	t.Helper()
	for _, command := range commands {
		if command.Command != "curl" || len(command.Args) != 4 || command.Args[1] != url || command.Args[2] != "-o" {
			continue
		}
		return command.Args[3]
	}
	t.Fatalf("no curl destination for %s in %#v", url, commands)
	return ""
}

func assertInstalledFrom(t *testing.T, commands []runner.Command, source, destination string) {
	t.Helper()
	assertHasCommand(t, commands, "install", "-m", "0644", source, destination)
}

func assertInstallDestination(t *testing.T, commands []runner.Command, destination string) {
	t.Helper()
	for _, command := range commands {
		if command.Command == "install" && len(command.Args) > 0 && command.Args[len(command.Args)-1] == destination {
			return
		}
	}
	t.Fatalf("commands %#v do not install %s", commands, destination)
}

func installedSource(t *testing.T, commands []runner.Command, destination string) string {
	t.Helper()
	for _, command := range commands {
		if command.Command == "install" && len(command.Args) >= 2 && command.Args[len(command.Args)-1] == destination {
			return command.Args[len(command.Args)-2]
		}
	}
	t.Fatalf("commands %#v do not install %s", commands, destination)
	return ""
}

func assertTemporaryRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file %s cleanup error = %v", path, err)
	}
}

type fixtureElevation struct {
	fixture *runner.Fixture
	calls   int
}

func (e *fixtureElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	e.calls++
	return e.fixture.Run(ctx, "sudo", append([]string{command}, args...)...)
}

type fileCapturingRunner struct {
	fixture *runner.Fixture
	files   map[string]string
}

func (r *fileCapturingRunner) LookPath(ctx context.Context, name string) (string, error) {
	return r.fixture.LookPath(ctx, name)
}

func (r *fileCapturingRunner) Run(ctx context.Context, command string, args ...string) (runner.Result, error) {
	if command == "install" && len(args) >= 2 && args[len(args)-2] != "-d" {
		if content, err := os.ReadFile(args[len(args)-2]); err == nil {
			r.files[args[len(args)-1]] = string(content)
		}
	}
	return r.fixture.Run(ctx, command, args...)
}

type installerFailureRunner struct {
	fixture    *runner.Fixture
	err        error
	status     int
	namePrefix string
}

func (r *installerFailureRunner) LookPath(ctx context.Context, name string) (string, error) {
	return r.fixture.LookPath(ctx, name)
}

func (r *installerFailureRunner) Run(ctx context.Context, command string, args ...string) (runner.Result, error) {
	result, err := r.fixture.Run(ctx, command, args...)
	if err != nil {
		return result, err
	}
	if command == "env" {
		for _, arg := range args {
			if strings.HasPrefix(filepath.Base(arg), r.namePrefix) {
				return runner.Result{ExitCode: r.status}, r.err
			}
		}
	}
	return result, nil
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want || strings.Contains(value, want) {
			return true
		}
	}
	return false
}
