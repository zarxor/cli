package adapters

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zarxor/scripts/internal/detect"
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

func TestWindowsUsesElevationOnlyForSystemChanges(t *testing.T) {
	fixture := runner.NewFixture()
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
	assertHasCommand(t, fixture.Commands, "npm", "install", "--global", "@openai/codex")
	for _, command := range fixture.Commands {
		if command.Command == "bash" || command.Command == "sh" || command.Command == "nvm" {
			t.Fatalf("Windows adapter invoked a Linux command: %#v", command)
		}
	}
}

func TestWindowsInstallerFailurePreservesExitStatus(t *testing.T) {
	fixture := runner.NewFixture()
	wantErr := &windowsExitError{status: 23}
	fixture.Set("npm", []string{"install", "--global", "@openai/codex"}, runner.Result{ExitCode: 23}, wantErr)
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
