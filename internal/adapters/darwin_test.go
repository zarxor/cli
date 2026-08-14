package adapters

import (
	"context"
	"reflect"
	"testing"

	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
)

func TestDarwinDetectsHomebrewToolAndCandidate(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "/opt/homebrew/bin/git"
	fixture.LookPaths["brew"] = "/opt/homebrew/bin/brew"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.49.0\n"}, nil)
	fixture.Set("brew", []string{"info", "--json=v2", "git"}, runner.Result{Stdout: `{"formulae":[{"versions":{"stable":"2.50.0"}}]}`}, nil)
	adapter := NewDarwinAdapter(fixture, fixture)

	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if !detection.Installed || detection.Current != "git version 2.49.0" || detection.Candidate != "2.50.0" {
		t.Fatalf("detection = %#v", detection)
	}
}

func TestDarwinInstallsDockerDesktopAndReusesItForComponents(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["brew"] = "/opt/homebrew/bin/brew"
	fixture.Set("brew", []string{"install", "--cask", "docker"}, runner.Result{}, nil)
	fixture.Set("brew", []string{"upgrade", "--cask", "docker"}, runner.Result{}, nil)
	adapter := NewDarwinAdapter(fixture, fixture)

	if err := adapter.Install(context.Background(), mustTool(t, profile.Docker)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Install(context.Background(), mustTool(t, profile.DockerBuildx)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.DockerCompose)); err != nil {
		t.Fatal(err)
	}
	want := []runner.Command{{Command: "brew", Args: []string{"install", "--cask", "docker"}}}
	if !reflect.DeepEqual(fixture.Commands, want) {
		t.Fatalf("commands = %#v, want %#v", fixture.Commands, want)
	}
}

func TestDarwinInstallsOpenCodeThroughOfficialNPMPackage(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("npm", []string{"install", "--global", "opencode-ai@latest"}, runner.Result{}, nil)
	adapter := NewDarwinAdapter(fixture, fixture)
	if err := adapter.Install(context.Background(), mustTool(t, profile.OpenCode)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 || !reflect.DeepEqual(fixture.Commands[0].Args, []string{"install", "--global", "opencode-ai@latest"}) {
		t.Fatalf("commands = %#v", fixture.Commands)
	}
}
