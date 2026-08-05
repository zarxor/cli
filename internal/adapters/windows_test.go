package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zarxor/scripts/internal/detect"
	"github.com/zarxor/scripts/internal/plan"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/runner"
)

func TestWindowsInstallsSystemToolsThroughWinGet(t *testing.T) {
	cases := []struct {
		id   profile.ToolID
		want []string
	}{
		{profile.Git, []string{"install", "--id", "Git.Git", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.GitHubCLI, []string{"install", "--id", "GitHub.cli", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.Docker, []string{"install", "--id", "Docker.DockerDesktop", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.NVM, []string{"install", "--id", "CoreyButler.NVMforWindows", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
	}

	for _, test := range cases {
		t.Run(string(test.id), func(t *testing.T) {
			fixture := runner.NewFixture()
			elevation := &windowsFixtureElevation{fixture: fixture}
			adapter := NewWindowsAdapter(fixture, elevation)

			if err := adapter.Install(context.Background(), mustTool(t, test.id)); err != nil {
				t.Fatal(err)
			}
			if len(elevation.commands) != 1 {
				t.Fatalf("elevated commands = %#v, want one WinGet install", elevation.commands)
			}
			if got := elevation.commands[0]; got.Command != "winget" || !reflect.DeepEqual(got.Args, test.want) {
				t.Fatalf("WinGet command = %#v, want winget %#v", got, test.want)
			}
		})
	}
}

func TestWindowsDetectsMissingToolAndDoesNotUpdateIt(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("winget", []string{"show", "--id", "Git.Git", "--exact"}, runner.Result{Stdout: "Version: 2.49.0\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got != (detect.Detection{Candidate: "2.49.0"}) {
		t.Fatalf("Detect() = %#v, want missing tool with candidate", got)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	for _, command := range fixture.Commands {
		if command.Command == "winget" && len(command.Args) > 0 && command.Args[0] == "upgrade" {
			t.Fatalf("Update() upgraded missing tool: %#v", fixture.Commands)
		}
	}
}

func TestWindowsTreatsMissingDockerComponentAsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["docker"] = `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.Set(`C:\Program Files\Docker\Docker\resources\bin\docker.exe`, []string{"compose", "version"}, runner.Result{Stderr: "docker: 'compose' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerCompose))
	if err != nil {
		t.Fatal(err)
	}
	if got != (detect.Detection{Candidate: "4.42.0"}) {
		t.Fatalf("Detect() = %#v, want absent Compose with Docker Desktop candidate", got)
	}
}

func TestWindowsPreservesGenuineDockerDetectionFailure(t *testing.T) {
	fixture := runner.NewFixture()
	path := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = path
	wantErr := errors.New("access denied")
	fixture.Set(path, []string{"buildx", "version"}, runner.Result{Stderr: "access denied\n", ExitCode: 1}, wantErr)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	_, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerBuildx))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Detect() error = %v, want genuine failure", err)
	}
}

func TestWindowsDetectsCandidatesForInstalledUserTools(t *testing.T) {
	fixture := runner.NewFixture()
	paths := map[string]string{
		"nvm":  `C:\Users\johan\AppData\Roaming\nvm\nvm.exe`,
		"node": `C:\Program Files\nodejs\node.exe`, "npm": `C:\Program Files\nodejs\npm.cmd`,
		"corepack": `C:\Program Files\nodejs\corepack.cmd`, "pnpm": `C:\Program Files\nodejs\pnpm.cmd`,
		"yarn": `C:\Program Files\nodejs\yarn.cmd`, "codex": `C:\Program Files\nodejs\codex.cmd`,
		"bun": `C:\Program Files\nodejs\bun.exe`,
	}
	for executable, path := range paths {
		fixture.LookPaths[executable] = path
		fixture.Set(path, windowsSources[mustTool(t, profile.ToolID(executable)).ID].version, runner.Result{Stdout: "1.0.0\n"}, nil)
	}
	fixture.Set(paths["nvm"], []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS | OLD STABLE |\n| 26.1.0 | 24.5.0 | 22.1.0 |\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "CoreyButler.NVMforWindows", "--exact"}, runner.Result{Stdout: "Version: 1.2.2\n"}, nil)
	packages := map[profile.ToolID]string{
		profile.NPM: "npm", profile.Corepack: "corepack", profile.PNPM: "pnpm",
		profile.Yarn: "@yarnpkg/cli-dist", profile.Codex: "@openai/codex", profile.Bun: "bun",
	}
	for id, packageName := range packages {
		fixture.Set(paths["npm"], []string{"view", packageName, "version"}, runner.Result{Stdout: "2.0.0\n"}, nil)
		_ = id
	}
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})
	cases := []struct {
		id   profile.ToolID
		want string
	}{
		{profile.NVM, "1.2.2"}, {profile.Node, "24.5.0"}, {profile.NPM, "2.0.0"}, {profile.Corepack, "2.0.0"},
		{profile.PNPM, "2.0.0"}, {profile.Yarn, "2.0.0"}, {profile.Codex, "2.0.0"}, {profile.Bun, "2.0.0"},
	}
	for _, test := range cases {
		t.Run(string(test.id), func(t *testing.T) {
			got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Installed || got.Candidate != test.want {
				t.Fatalf("Detect() = %#v, want candidate %q", got, test.want)
			}
		})
	}
}

func TestFreshWindowsDevelopmentProfileConvergesProvidersOnceWithoutProcessPATHRefresh(t *testing.T) {
	programFiles := t.TempDir()
	nvmHome := filepath.Join(t.TempDir(), "nvm")
	nvmSymlink := filepath.Join(t.TempDir(), "nodejs")
	config := WindowsConfig{ProgramFiles: programFiles, NVMHome: nvmHome, NVMSymlink: nvmSymlink}
	paths := map[string]string{
		"git":    filepath.Join(programFiles, "Git", "cmd", "git.exe"),
		"gh":     filepath.Join(programFiles, "GitHub CLI", "gh.exe"),
		"docker": filepath.Join(programFiles, "Docker", "Docker", "resources", "bin", "docker.exe"),
		"nvm":    filepath.Join(nvmHome, "nvm.exe"), "node": filepath.Join(nvmSymlink, "node.exe"),
		"npm": filepath.Join(nvmSymlink, "npm.cmd"), "corepack": filepath.Join(nvmSymlink, "corepack.cmd"),
		"pnpm": filepath.Join(nvmSymlink, "pnpm.cmd"), "yarn": filepath.Join(nvmSymlink, "yarn.cmd"),
		"codex": filepath.Join(nvmSymlink, "codex.cmd"), "bun": filepath.Join(nvmSymlink, "bun.cmd"),
	}
	fixture := runner.NewFixture()
	for _, path := range paths {
		fixture.LookPaths[path] = path
	}
	for id, source := range windowsSources {
		path := paths[source.executable]
		fixture.Set(path, source.version, runner.Result{Stdout: string(id) + " 1.0.0\n"}, nil)
	}
	fixture.Set(paths["nvm"], []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS |\n| 26.1.0 | 24.5.0 |\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation, config)
	planned, err := plan.MergeProfiles([]profile.Profile{profile.DevelopmentProfile()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := plan.DependencyOrder(planned)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range ordered {
		if err := adapter.Install(context.Background(), tool); err != nil {
			t.Fatalf("Install(%s): %v", tool.ID, err)
		}
		if err := adapter.Verify(context.Background(), tool); err != nil {
			t.Fatalf("Verify(%s): %v", tool.ID, err)
		}
	}

	dockerInstalls := 0
	for _, command := range elevation.commands {
		if command.Command == "winget" && slicesContain(command.Args, "Docker.DockerDesktop") && command.Args[0] == "install" {
			dockerInstalls++
		}
	}
	if dockerInstalls != 1 {
		t.Fatalf("Docker Desktop install count = %d, want 1; commands = %#v", dockerInstalls, elevation.commands)
	}
	for _, command := range fixture.Commands {
		switch command.Command {
		case "git", "gh", "docker", "nvm", "node", "npm", "corepack", "pnpm", "yarn", "codex", "bun":
			t.Fatalf("fresh profile relied on stale process PATH: %#v", command)
		}
	}
}

func TestWindowsUpdatesInstalledToolFromSameWinGetSource(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "C:\\Program Files\\Git\\cmd\\git.exe"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Git.Git", "--exact"}, runner.Result{Stdout: "Version: 2.49.0\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	if len(elevation.commands) != 1 {
		t.Fatalf("elevated commands = %#v, want one WinGet upgrade", elevation.commands)
	}
	want := []string{"upgrade", "--id", "Git.Git", "--exact", "--accept-package-agreements", "--accept-source-agreements"}
	if got := elevation.commands[0]; got.Command != "winget" || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("WinGet command = %#v, want winget %#v", got, want)
	}
}

func TestWindowsDefaultElevationUsesFileHelperWithSeparateArguments(t *testing.T) {
	fixture := runner.NewFixture()
	elevation := windowsElevation{runner: fixture}
	wingetArgs := []string{"install", "--id", "Git.Git", "--exact"}

	if _, err := elevation.RunElevated(context.Background(), "winget", wingetArgs...); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 {
		t.Fatalf("commands = %#v, want one PowerShell invocation", fixture.Commands)
	}
	got := fixture.Commands[0]
	wantPrefix := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
	if got.Command != "powershell.exe" || len(got.Args) < len(wantPrefix)+2 || !reflect.DeepEqual(got.Args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("default elevation = %#v, want PowerShell -File helper", got)
	}
	helper := got.Args[len(wantPrefix)]
	wantPassedArgs := append([]string{"winget"}, wingetArgs...)
	if !reflect.DeepEqual(got.Args[len(wantPrefix)+1:], wantPassedArgs) {
		t.Fatalf("PowerShell arguments = %#v, want %#v", got.Args[len(wantPrefix)+1:], wantPassedArgs)
	}
	for _, arg := range got.Args {
		if arg == "-Command" {
			t.Fatalf("default elevation used -Command instead of -File: %#v", got)
		}
	}
	if _, err := os.Stat(helper); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("elevation helper cleanup error = %v", err)
	}
}

func TestWindowsNodeInstallAndUpdateUseElevatedNVMSequence(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*runner.Fixture)
		run     func(Adapter) error
	}{
		{
			name: "install",
			run: func(adapter Adapter) error {
				return adapter.Install(context.Background(), mustTool(t, profile.Node))
			},
		},
		{
			name: "update",
			prepare: func(fixture *runner.Fixture) {
				fixture.LookPaths["node"] = "C:\\Program Files\\nodejs\\node.exe"
				fixture.Set("node", []string{"--version"}, runner.Result{Stdout: "v24.0.0\n"}, nil)
			},
			run: func(adapter Adapter) error {
				return adapter.Update(context.Background(), mustTool(t, profile.Node))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			nvmPath := `C:\Users\johan\AppData\Roaming\nvm\nvm.exe`
			fixture.LookPaths["nvm"] = nvmPath
			fixture.Set(nvmPath, []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS |\n| 26.1.0 | 24.5.0 |\n"}, nil)
			if test.prepare != nil {
				test.prepare(fixture)
			}
			elevation := &windowsFixtureElevation{fixture: fixture}

			if err := test.run(NewWindowsAdapter(fixture, elevation)); err != nil {
				t.Fatal(err)
			}
			want := []runner.Command{
				{Command: nvmPath, Args: []string{"install", "lts"}},
				{Command: nvmPath, Args: []string{"use", "lts"}},
			}
			if !reflect.DeepEqual(elevation.commands, want) {
				t.Fatalf("elevated commands = %#v, want %#v", elevation.commands, want)
			}
		})
	}
}

func TestWindowsUsesElevationOnlyForSystemChanges(t *testing.T) {
	fixture := runner.NewFixture()
	npmPath := `C:\Program Files\nodejs\npm.cmd`
	fixture.LookPaths["npm"] = npmPath
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	if err := adapter.Install(context.Background(), mustTool(t, profile.Codex)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}
	if len(elevation.commands) != 0 {
		t.Fatalf("user installers used elevation: %#v", elevation.commands)
	}
	assertHasCommand(t, fixture.Commands, npmPath, "install", "--global", "@openai/codex@latest")
	assertHasCommand(t, fixture.Commands, npmPath, "install", "--global", "bun@latest")
	for _, command := range fixture.Commands {
		if command.Command == "bash" || command.Command == "sh" || command.Command == "nvm" {
			t.Fatalf("Windows adapter invoked a Linux command: %#v", command)
		}
	}
}

func TestWindowsInstallerFailurePreservesExitStatus(t *testing.T) {
	fixture := runner.NewFixture()
	wantErr := &windowsExitError{status: 23}
	npmPath := `C:\Program Files\nodejs\npm.cmd`
	fixture.LookPaths["npm"] = npmPath
	fixture.Set(npmPath, []string{"install", "--global", "@openai/codex@latest"}, runner.Result{ExitCode: 23}, wantErr)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	err := adapter.Install(context.Background(), mustTool(t, profile.Codex))
	if !errors.Is(err, wantErr) || err.(*windowsExitError).ExitCode() != 23 {
		t.Fatalf("Install() error = %#v, want unchanged exit status 23", err)
	}
}

func TestWindowsRejectsToolsWithoutNativeInstallationPath(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	err := adapter.Install(context.Background(), profile.Tool{ID: "linux-only", Name: "Linux only"})
	if err == nil {
		t.Fatal("Install() error = nil, want unsupported-provider error")
	}
}

type windowsFixtureElevation struct {
	fixture  *runner.Fixture
	commands []runner.Command
}

func (e *windowsFixtureElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	e.commands = append(e.commands, runner.Command{Command: command, Args: append([]string(nil), args...)})
	return e.fixture.Run(ctx, command, args...)
}

type windowsExitError struct{ status int }

func (e *windowsExitError) Error() string { return "Windows installer failed" }
func (e *windowsExitError) ExitCode() int { return e.status }
