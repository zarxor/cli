package profile_test

import (
	"reflect"
	"testing"

	"github.com/zarxor/cli/internal/profile"
)

func TestDevelopmentProfileContainsDevelopmentTools(t *testing.T) {
	got := profile.DevelopmentProfile()
	want := profile.Profile{
		Name: profile.Development,
		ToolIDs: []profile.ToolID{
			profile.Git,
			profile.GitHubCLI,
			profile.Docker,
			profile.Claude,
			profile.Codex,
			profile.Node,
			profile.T3Code,
			profile.Bun,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DevelopmentProfile() = %#v, want %#v", got, want)
	}
}

func TestDesktopProfileContainsTheLocalDevelopmentToolchain(t *testing.T) {
	got := profile.DesktopProfile()
	want := []profile.ToolID{
		profile.Git,
		profile.GitHubCLI,
		profile.Docker,
		profile.Claude,
		profile.Codex,
		profile.Node,
		profile.T3Code,
		profile.Bun,
	}
	if got.Name != profile.Desktop || !reflect.DeepEqual(got.ToolIDs, want) {
		t.Fatalf("DesktopProfile() = %#v, want name %q and tools %v", got, profile.Desktop, want)
	}
}

func TestServerProfileIncludesAgentCLIsAndLeavesDesktopOnlyToolsOut(t *testing.T) {
	got := profile.ServerProfile()
	want := []profile.ToolID{profile.Git, profile.GitHubCLI, profile.Docker, profile.Claude, profile.Codex, profile.Node}
	if got.Name != profile.Server || !reflect.DeepEqual(got.ToolIDs, want) {
		t.Fatalf("ServerProfile() = %#v, want name %q and tools %v", got, profile.Server, want)
	}
}

func TestResolveProfilesAcceptsAutomaticProfiles(t *testing.T) {
	got, err := profile.ResolveProfiles([]string{"desktop", "server"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != profile.Desktop || got[1].Name != profile.Server {
		t.Fatalf("ResolveProfiles() = %#v, want desktop and server", got)
	}
}

func TestResolveProfilesRejectsUnknownProfile(t *testing.T) {
	_, err := profile.ResolveProfiles([]string{"does-not-exist"})
	if err == nil {
		t.Fatal("ResolveProfiles() error = nil, want an unknown-profile error")
	}
}
