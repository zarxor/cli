package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
	"github.com/zarxor/cli/internal/tools"
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

func TestDebianDockerComponentsReuseRepositoryMetadata(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: t.TempDir(), TempDir: t.TempDir(), Distribution: "debian",
		Codename: "bookworm", Architecture: "amd64",
	})

	for _, id := range []profile.ToolID{profile.Docker, profile.DockerBuildx, profile.DockerCompose} {
		if err := adapter.Install(context.Background(), mustTool(t, id)); err != nil {
			t.Fatalf("Install(%s): %v", id, err)
		}
	}

	updates := 0
	for _, command := range fixture.Commands {
		if command.Command == "apt-get" && len(command.Args) == 1 && command.Args[0] == "update" {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("apt metadata refreshes = %d, want one shared refresh; commands = %#v", updates, fixture.Commands)
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

func TestArchBunUsesIntegrityCheckedNPMProviderWithoutSystemMutation(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	adapter := NewArchAdapter(fixture, &fixtureElevation{fixture: fixture}, LinuxConfig{Root: false, Home: home, TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "NVM_DIR="+filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", filepath.Join(home, ".nvm", "nvm-exec"), "npm", "install", "--global", "bun@latest")
	for _, command := range fixture.Commands {
		if command.Command == "sudo" || command.Command == "curl" {
			t.Fatalf("Bun package install used an elevated or remote-script command: %#v", command)
		}
	}
}

func TestDebianBunUsesIntegrityCheckedNPMProvider(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	adapter := NewDebianAdapter(fixture, &fixtureElevation{fixture: fixture}, LinuxConfig{Root: false, Home: home, TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "NVM_DIR="+filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", filepath.Join(home, ".nvm", "nvm-exec"), "npm", "install", "--global", "bun@latest")
}

func TestLinuxUserToolInstallsAvoidUnverifiedRemoteScripts(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

	for _, id := range []profile.ToolID{profile.Codex, profile.NVM, profile.Bun} {
		if err := adapter.Install(context.Background(), mustTool(t, id)); err != nil {
			t.Fatal(err)
		}
	}

	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "NVM_DIR="+filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "install", "--global", "@openai/codex@latest")
	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "git", "clone", "--branch", "v0.40.3", "--depth", "1", "https://github.com/nvm-sh/nvm.git", filepath.Join(home, ".nvm"))
	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "git", "-C", filepath.Join(home, ".nvm"), "checkout", "--detach", "d025499c7f5466d0dc0a324dc98eab72cce8377d")
	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "NVM_DIR="+filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "install", "--global", "bun@latest")
	for _, command := range fixture.Commands {
		if command.Command == "curl" {
			t.Fatalf("user tool install downloaded an executable script: %#v", command)
		}
	}
}

func TestSudoInvocationDelegatesCompleteProfileMutationToInvokingUser(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	profilePath := filepath.Join(home, ".bashrc")
	original := "export KEEP=1\n"
	if err := os.WriteFile(profilePath, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	rootTemp := t.TempDir()
	config := LinuxConfig{
		Root: true, Home: home, TempDir: rootTemp,
		InvokingUser: "johan", InvokingUID: 1000, InvokingGID: 1000,
	}
	adapter := NewArchAdapter(fixture, fixture, config)

	if err := adapter.Install(context.Background(), mustTool(t, profile.NVM)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "sudo", "-H", "-u", "johan", "env", "HOME="+home, "git", "clone", "--branch", "v0.40.3", "--depth", "1", "https://github.com/nvm-sh/nvm.git", filepath.Join(home, ".nvm"))
	assertHasUserProfileHelper(t, fixture.Commands, home, rootTemp, profilePath, "nvm")
	for _, command := range fixture.Commands {
		if command.Command == "chown" {
			t.Fatalf("profile ownership was repaired after a root mutation: %#v", command)
		}
	}
	got, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("root process mutated profile directly: got %q, want untouched %q", got, original)
	}
	if info, err := os.Stat(profilePath); err != nil || info.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Fatalf("root process changed profile mode: info=%v err=%v", info, err)
	}
}

func TestSudoProfileMutationDoesNotCopyExistingProfileIntoRootTemporary(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	rootTemp := t.TempDir()
	profilePath := filepath.Join(home, ".bashrc")
	privateProfile := "export PRIVATE_API_TOKEN=do-not-copy-as-root\n"
	if err := os.WriteFile(profilePath, []byte(privateProfile), 0640); err != nil {
		t.Fatal(err)
	}
	userTemporary := filepath.Join(home, ".jb-profile-user")
	userPrefix := []string{"-H", "-u", "johan", "env", "HOME=" + home}
	fixture.Set("sudo", append(append([]string{}, userPrefix...), "stat", "-c", "%a", "--", profilePath), runner.Result{Stdout: "640\n"}, nil)
	fixture.Set("sudo", append(append([]string{}, userPrefix...), "cat", "--", profilePath), runner.Result{Stdout: privateProfile}, nil)
	fixture.Set("sudo", append(append([]string{}, userPrefix...), "mktemp", filepath.Join(home, ".jb-profile-XXXXXX")), runner.Result{Stdout: userTemporary + "\n"}, nil)
	capturing := &rootTemporaryCapturingRunner{fixture: fixture, directory: rootTemp, contents: make(map[string]string)}
	adapter := NewArchAdapter(capturing, fixture, LinuxConfig{
		Root: true, Home: home, TempDir: rootTemp, InvokingUser: "johan", InvokingUID: 1000, InvokingGID: 1000,
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.NVM)); err != nil {
		t.Fatal(err)
	}
	for path, content := range capturing.contents {
		if strings.Contains(content, privateProfile) {
			t.Fatalf("root temporary %s copied full private profile content: %q", path, content)
		}
	}
}

func TestSudoProfileMutationDoesNotParseLocalizedMissingFileDiagnostics(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	rootTemp := t.TempDir()
	profilePath := filepath.Join(home, ".bashrc")
	userPrefix := []string{"-H", "-u", "johan", "env", "HOME=" + home}
	translatedErr := errors.New("exit status 1")
	fixture.Set("sudo", append(append([]string{}, userPrefix...), "stat", "-c", "%a", "--", profilePath), runner.Result{Stderr: "stat: kan inte ta status på filen\n", ExitCode: 1}, translatedErr)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{
		Root: true, Home: home, TempDir: rootTemp, InvokingUser: "johan", InvokingUID: 1000, InvokingGID: 1000,
	})

	if err := adapter.Install(context.Background(), mustTool(t, profile.NVM)); err != nil {
		t.Fatalf("Install() error = %v, want absent profile handled without localized stderr parsing", err)
	}
	assertHasUserProfileHelper(t, fixture.Commands, home, rootTemp, profilePath, "nvm")
	for _, command := range fixture.Commands {
		if command.Command == "sudo" && slicesContain(command.Args, "stat") && slicesContain(command.Args, profilePath) {
			t.Fatalf("root-side transaction still parsed localized stat diagnostics: %#v", command)
		}
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

func TestDebianRefusesToUpdateManualSystemInstallation(t *testing.T) {
	fixture := runner.NewFixture()
	path := "/usr/local/bin/git"
	fixture.LookPaths["git"] = path
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	fixture.Set("dpkg-query", []string{"-S", path}, runner.Result{Stdout: "local-git: /usr/local/bin/git\n"}, nil)
	fixture.Set("apt-cache", []string{"policy", "git"}, runner.Result{Stdout: "Candidate: 2.49.0-1\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if !detection.Installed || detection.Candidate != "" {
		t.Fatalf("Detect() = %#v, want installed manual Git without update candidate", detection)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err == nil {
		t.Fatal("Update() error = nil, want provider ownership refusal")
	}
	for _, command := range fixture.Commands {
		if command.Command == "apt-get" && len(command.Args) > 0 && command.Args[0] == "install" {
			t.Fatalf("manual Git triggered apt update: %#v", fixture.Commands)
		}
	}
}

func TestDebianTreatsMissingDockerComponentAsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["docker"] = "/usr/bin/docker"
	fixture.Set("docker", []string{"buildx", "version"}, runner.Result{Stderr: "docker: 'buildx' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
	fixture.Set("apt-cache", []string{"policy", "docker-buildx-plugin"}, runner.Result{Stdout: "Candidate: 0.24.0-1\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerBuildx))
	if err != nil {
		t.Fatal(err)
	}
	if got != (detect.Detection{Installed: false, Candidate: "0.24.0-1"}) {
		t.Fatalf("Detect() = %#v, want absent Buildx with package candidate", got)
	}
}

func TestDebianPreservesGenuineDockerDetectionFailure(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["docker"] = "/usr/bin/docker"
	wantErr := errors.New("permission denied")
	fixture.Set("docker", []string{"compose", "version"}, runner.Result{Stderr: "permission denied\n", ExitCode: 1}, wantErr)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	_, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerCompose))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Detect() error = %v, want genuine command failure", err)
	}
}

func TestLinuxTreatsMissingNVMManagedComponentAsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture.LookPaths[nvmExec] = nvmExec
	args := []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "pnpm", "--version"}
	fixture.Set("env", args, runner.Result{Stderr: nvmExec + ": line 20: exec: pnpm: not found\n", ExitCode: 127}, errors.New("exit status 127"))
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.PNPM))
	if err != nil {
		t.Fatal(err)
	}
	if got.Installed {
		t.Fatalf("Detect() = %#v, want absent pnpm", got)
	}
}

func TestLinuxBrokenPresentExecutablesRemainDetectionErrors(t *testing.T) {
	tests := []struct {
		name    string
		result  runner.Result
		wantErr error
	}{
		{name: "dynamic loader failure", result: runner.Result{Stderr: "npm: error while loading shared libraries: libnode.so: cannot open shared object file: No such file or directory\n", ExitCode: 127}, wantErr: errors.New("loader failed")},
		{name: "permission failure", result: runner.Result{Stderr: "npm: Permission denied\n", ExitCode: 127}, wantErr: errors.New("permission denied")},
		{name: "arbitrary missing file", result: runner.Result{Stderr: "npm: config: No such file or directory\n", ExitCode: 1}, wantErr: errors.New("config missing")},
		{name: "embedded module diagnostic", result: runner.Result{Stderr: "module loader failure: npm: not found\n", ExitCode: 127}, wantErr: errors.New("module failed")},
		{name: "embedded permission diagnostic", result: runner.Result{Stderr: "permission denied: npm: command not found\n", ExitCode: 127}, wantErr: errors.New("permission failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
			fixture := runner.NewFixture()
			fixture.LookPaths[nvmExec] = nvmExec
			fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "--version"}, test.result, test.wantErr)
			adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

			_, err := adapter.Detect(context.Background(), mustTool(t, profile.NPM))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Detect() error = %v, want broken-present error %v", err, test.wantErr)
			}
		})
	}
}

func TestLinuxDetectsCandidatesForInstalledUserTools(t *testing.T) {
	home := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture := runner.NewFixture()
	fixture.LookPaths[nvmExec] = nvmExec
	fixture.Set("git", []string{"ls-remote", "--tags", "--refs", "https://github.com/nvm-sh/nvm.git", "v*"}, runner.Result{Stdout: "aaa\trefs/tags/v0.40.3\nbbb\trefs/tags/v0.40.4\n"}, nil)
	fixture.Set("curl", []string{"-fsSL", "https://nodejs.org/dist/index.json"}, runner.Result{Stdout: `[{"version":"v26.1.0","lts":false},{"version":"v24.5.0","lts":"Krypton"}]`}, nil)

	packages := map[profile.ToolID]string{
		profile.NPM: "npm", profile.Corepack: "corepack", profile.PNPM: "pnpm",
		profile.Yarn: "@yarnpkg/cli-dist", profile.Codex: "@openai/codex", profile.Bun: "bun",
	}
	for id, packageName := range packages {
		executable, _ := nvmExecutable(id)
		currentArgs := []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, executable, "--version"}
		fixture.Set("env", currentArgs, runner.Result{Stdout: "1.0.0\n"}, nil)
		candidateArgs := []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "view", packageName, "version"}
		fixture.Set("env", candidateArgs, runner.Result{Stdout: "2.0.0\n"}, nil)
	}
	fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "node", "--version"}, runner.Result{Stdout: "v22.0.0\n"}, nil)

	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})
	cases := []struct {
		id   profile.ToolID
		want string
	}{
		{profile.NVM, "v0.40.4"}, {profile.Node, "v24.5.0"},
		{profile.NPM, "2.0.0"}, {profile.Corepack, "2.0.0"},
		{profile.PNPM, "2.0.0"}, {profile.Yarn, "2.0.0"},
		{profile.Codex, "2.0.0"}, {profile.Bun, "2.0.0"},
	}
	for _, test := range cases {
		t.Run(string(test.id), func(t *testing.T) {
			got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Installed || got.Candidate != test.want {
				t.Fatalf("Detect() = %#v, want installed candidate %q", got, test.want)
			}
		})
	}
}

func TestLinuxCandidateLookupFailuresPreserveSuccessfulDetection(t *testing.T) {
	tests := []struct {
		name         string
		id           profile.ToolID
		executable   string
		current      string
		setCandidate func(*runner.Fixture, string)
	}{
		{
			name: "malformed Node metadata", id: profile.Node, executable: "node", current: "v22.1.0",
			setCandidate: func(fixture *runner.Fixture, _ string) {
				fixture.Set("curl", []string{"-fsSL", "https://nodejs.org/dist/index.json"}, runner.Result{Stdout: "not-json"}, nil)
			},
		},
		{
			name: "npm registry outage", id: profile.NPM, executable: "npm", current: "10.8.0",
			setCandidate: func(fixture *runner.Fixture, nvmExec string) {
				fixture.Set("env", []string{"HOME=" + filepath.Dir(filepath.Dir(nvmExec)), "NVM_DIR=" + filepath.Dir(nvmExec), "NODE_VERSION=lts/*", nvmExec, "npm", "view", "npm", "version"}, runner.Result{}, errors.New("registry unavailable"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
			fixture := runner.NewFixture()
			fixture.LookPaths[nvmExec] = nvmExec
			fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, test.executable, "--version"}, runner.Result{Stdout: test.current + "\n"}, nil)
			test.setCandidate(fixture, nvmExec)
			adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

			got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if err != nil {
				t.Fatalf("Detect() error = %v, want candidate-only failure ignored", err)
			}
			if got != (detect.Detection{Installed: true, Current: test.current}) {
				t.Fatalf("Detect() = %#v, want installed/current preserved with blank candidate", got)
			}
			if err := adapter.Verify(context.Background(), mustTool(t, test.id)); err != nil {
				t.Fatalf("Verify() error = %v, want candidate-only failure ignored", err)
			}
		})
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

func TestArchRefusesToUpdateManualSystemInstallation(t *testing.T) {
	fixture := runner.NewFixture()
	path := "/usr/local/bin/git"
	fixture.LookPaths["git"] = path
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	fixture.Set("pacman", []string{"-Qo", path}, runner.Result{Stdout: "error: No package owns /usr/local/bin/git\n"}, nil)
	fixture.Set("pacman", []string{"-Si", "git"}, runner.Result{Stdout: "Version         : 2.49.0-1\n"}, nil)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if !detection.Installed || detection.Candidate != "" {
		t.Fatalf("Detect() = %#v, want installed manual Git without update candidate", detection)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err == nil {
		t.Fatal("Update() error = nil, want provider ownership refusal")
	}
	for _, command := range fixture.Commands {
		if command.Command == "pacman" && len(command.Args) > 0 && command.Args[0] == "-Syu" {
			t.Fatalf("manual Git triggered pacman update: %#v", fixture.Commands)
		}
	}
}

func TestDebianVerifySkipsCandidateLookup(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "/usr/bin/git"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: t.TempDir(), TempDir: t.TempDir()})

	if err := adapter.Verify(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	for _, command := range fixture.Commands {
		if command.Command == "apt-cache" {
			t.Fatalf("Verify() performed candidate lookup: %#v", fixture.Commands)
		}
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

func TestCodexPackageInstallDoesNotTouchPredictableScriptPath(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	home := t.TempDir()
	predictable := filepath.Join(temp, "attacker-owned-marker")
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
	for _, command := range fixture.Commands {
		if command.Command == "curl" {
			t.Fatalf("Codex package install downloaded a script: %#v", command)
		}
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

func TestNVMManagedCommandsForceLTSDespiteProjectNVMRC(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".nvmrc"), []byte("v18.20.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

	if err := adapter.Install(context.Background(), mustTool(t, profile.Codex)); err != nil {
		t.Fatal(err)
	}

	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	for _, command := range fixture.Commands {
		if command.Command != "env" || !slicesContain(command.Args, nvmExec) {
			continue
		}
		if !slicesContain(command.Args, "NODE_VERSION=lts/*") {
			t.Fatalf("NVM-managed command = %#v, want explicit LTS NODE_VERSION despite project %s", command, project)
		}
		return
	}
	t.Fatalf("commands = %#v, want NVM-managed command", fixture.Commands)
}

func TestNodeConvergenceUpdatesPersistentNVMDefault(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(Adapter) error
	}{{
		name: "install",
		run: func(adapter Adapter) error {
			return adapter.Install(context.Background(), mustTool(t, profile.Node))
		},
	}, {
		name: "update",
		run: func(adapter Adapter) error {
			return adapter.Update(context.Background(), mustTool(t, profile.Node))
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			home := t.TempDir()
			if test.name == "update" {
				fixture.Set("git", []string{"ls-remote", "--tags", "--refs", "https://github.com/nvm-sh/nvm.git", "v*"}, runner.Result{Stdout: "aaa\trefs/tags/v0.40.3\n"}, nil)
			}
			adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: t.TempDir()})

			if err := test.run(adapter); err != nil {
				t.Fatal(err)
			}
			aliasCount := 0
			for _, command := range fixture.Commands {
				if slicesContain(command.Args, "alias") && slicesContain(command.Args, "default") && slicesContain(command.Args, "lts/*") {
					aliasCount++
				}
			}
			if aliasCount != 1 {
				t.Fatalf("NVM default alias commands = %d; commands = %#v, want one persistent LTS alias", aliasCount, fixture.Commands)
			}
		})
	}
}

func TestCleanProcessDetectsCodexAndNodeToolsFromConfiguredHome(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	temp := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture.LookPaths[nvmExec] = nvmExec
	fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "codex", "--version"}, runner.Result{Stdout: "codex-cli 1.2.3\n"}, nil)
	fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "node", "--version"}, runner.Result{Stdout: "v24.1.0\n"}, nil)
	fixture.Set("env", []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "view", "@openai/codex", "version"}, runner.Result{Stdout: "1.2.4\n"}, nil)
	fixture.Set("curl", []string{"-fsSL", "https://nodejs.org/dist/index.json"}, runner.Result{Stdout: `[{"version":"v24.1.0","lts":"Krypton"}]`}, nil)
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

func TestUserPackageFailurePreservesExitStatus(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	wantErr := &fixtureExitError{status: 23}
	home := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	args := []string{"HOME=" + home, "NVM_DIR=" + filepath.Join(home, ".nvm"), "NODE_VERSION=lts/*", nvmExec, "npm", "install", "--global", "@openai/codex@latest"}
	fixture.Set("env", args, runner.Result{ExitCode: 23}, wantErr)
	adapter := NewDebianAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	err := adapter.Install(context.Background(), mustTool(t, profile.Codex))
	if !errors.Is(err, wantErr) || err.(*fixtureExitError).ExitCode() != 23 {
		t.Fatalf("Install() error = %#v, want unchanged exit status 23", err)
	}
}

func TestNVMUpdateResolvesLatestStableReleaseWithoutRemoteScript(t *testing.T) {
	fixture := runner.NewFixture()
	temp := t.TempDir()
	home := t.TempDir()
	fixture.Set("git", []string{"ls-remote", "--tags", "--refs", "https://github.com/nvm-sh/nvm.git", "v*"}, runner.Result{Stdout: "aaa\trefs/tags/v0.39.7\nbbb\trefs/tags/v0.40.3\nccc\trefs/tags/v0.40.10\n"}, nil)
	adapter := NewArchAdapter(fixture, fixture, LinuxConfig{Root: true, Home: home, TempDir: temp})

	if err := adapter.Update(context.Background(), mustTool(t, profile.NVM)); err != nil {
		t.Fatal(err)
	}

	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "git", "-C", filepath.Join(home, ".nvm"), "fetch", "--depth", "1", "origin", "tag", "v0.40.10")
	assertHasCommand(t, fixture.Commands, "env", "HOME="+home, "git", "-C", filepath.Join(home, ".nvm"), "checkout", "--detach", "v0.40.10")
	for _, command := range fixture.Commands {
		if command.Command == "curl" {
			t.Fatalf("NVM update downloaded an executable script: %#v", command)
		}
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

func assertHasUserProfileHelper(t *testing.T, commands []runner.Command, home, rootTemp, profilePath, blockName string) {
	t.Helper()
	prefix := []string{"-H", "-u", "johan", "env", "HOME=" + home, "sh"}
	for _, got := range commands {
		if got.Command != "sudo" || len(got.Args) != len(prefix)+4 || !reflect.DeepEqual(got.Args[:len(prefix)], prefix) {
			continue
		}
		helper := got.Args[len(prefix)]
		if filepath.Dir(helper) == rootTemp && got.Args[len(prefix)+1] == profilePath && got.Args[len(prefix)+2] == blockName {
			return
		}
	}
	t.Fatalf("commands %#v do not contain structured sudo-user profile helper for %s", commands, profilePath)
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

type rootTemporaryCapturingRunner struct {
	fixture   *runner.Fixture
	directory string
	contents  map[string]string
}

func (r *rootTemporaryCapturingRunner) LookPath(ctx context.Context, name string) (string, error) {
	return r.fixture.LookPath(ctx, name)
}

func (r *rootTemporaryCapturingRunner) Run(ctx context.Context, command string, args ...string) (runner.Result, error) {
	for _, arg := range args {
		relative, err := filepath.Rel(r.directory, arg)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if content, err := os.ReadFile(arg); err == nil {
			r.contents[arg] = string(content)
		}
	}
	return r.fixture.Run(ctx, command, args...)
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

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want || strings.Contains(value, want) {
			return true
		}
	}
	return false
}
