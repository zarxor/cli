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
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
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
	temp := t.TempDir()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: temp, Architecture: "amd64",
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.GitHubCLI)); err != nil {
		t.Fatal(err)
	}

	wantSource := "deb [arch=amd64 signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n"
	gotSource, err := os.ReadFile(filepath.Join(temp, "github-cli.list"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSource) != wantSource {
		t.Fatalf("repository definition = %q, want %q", gotSource, wantSource)
	}
	assertHasCommand(t, fixture.Commands, "curl", "-fsSL", "https://cli.github.com/packages/githubcli-archive-keyring.gpg", "-o", filepath.Join(temp, "githubcli-archive-keyring.gpg"))
	assertHasCommand(t, fixture.Commands, "install", "-m", "0644", filepath.Join(temp, "github-cli.list"), "/etc/apt/sources.list.d/github-cli.list")
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

	assertHasCommand(t, fixture.Commands, "curl", "-fsSL", "https://download.docker.com/linux/ubuntu/gpg", "-o", filepath.Join(temp, "docker.asc"))
	assertHasCommand(t, fixture.Commands, "install", "-m", "0644", filepath.Join(temp, "docker.sources"), "/etc/apt/sources.list.d/docker.sources")
}

func TestArchInstallAndUpdateUseNeededPacmanTransactions(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})
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
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "sudo", "pacman", "-Syu", "--noconfirm", "--needed", "unzip")
	assertHasCommand(t, fixture.Commands, "curl", "-fsSL", "https://bun.sh/install", "-o", filepath.Join(adapter.(*ArchAdapter).config.TempDir, "jb-bun-install.sh"))
	for _, command := range fixture.Commands {
		if command.Command == "sudo" && slicesContain(command.Args, "jb-bun-install.sh") {
			t.Fatalf("Bun installer was elevated: %#v", command)
		}
	}
}

func TestDebianBunInstallsUnzipBeforeUserInstaller(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: false, Home: t.TempDir(), TempDir: t.TempDir()})

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

func TestInstallerFailurePreservesExitStatusAndCleansTemporaryFile(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	installer := filepath.Join(temp, "jb-codex-install.sh")
	wantErr := &fixtureExitError{status: 23}
	home := t.TempDir()
	fixture.Set("env", []string{"HOME=" + home, "sh", installer}, runner.Result{ExitCode: 23}, wantErr)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	err := adapter.Install(context.Background(), mustTool(t, profile.Codex))
	if !errors.Is(err, wantErr) || err.(*fixtureExitError).ExitCode() != 23 {
		t.Fatalf("Install() error = %#v, want unchanged exit status 23", err)
	}
	if _, statErr := os.Stat(installer); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("installer cleanup error = %v", statErr)
	}
	assertHasCommand(t, fixture.Commands, "curl", "-fsSL", "https://chatgpt.com/codex/install.sh", "-o", installer)
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

	installer := filepath.Join(temp, "jb-nvm-install.sh")
	assertHasCommand(t, fixture.Commands, "curl", "-fsSL", "https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.10/install.sh", "-o", installer)
	if _, err := os.Stat(installer); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installer cleanup error = %v", err)
	}
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

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want || strings.Contains(value, want) {
			return true
		}
	}
	return false
}
