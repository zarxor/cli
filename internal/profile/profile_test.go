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

	if got.Name != want.Name || !reflect.DeepEqual(got.ToolIDs, want.ToolIDs) {
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

func TestResolveProfilesAcceptsComposableProfiles(t *testing.T) {
	got, err := profile.ResolveProfiles([]string{"agents", "python", "optional"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("profiles = %#v, want three profiles", got)
	}
	if !containsAll(got[0].ToolIDs, profile.Claude, profile.Codex, profile.T3Code) {
		t.Fatalf("agents tools = %v", got[0].ToolIDs)
	}
	if !containsAll(got[1].ToolIDs, profile.UV) {
		t.Fatalf("python tools = %v", got[1].ToolIDs)
	}
	if !containsAll(got[2].ToolIDs, profile.Mise, profile.UV, profile.OpenCode) {
		t.Fatalf("optional tools = %v", got[2].ToolIDs)
	}
}

func TestDevelopmentProfileComposesFocusedProfiles(t *testing.T) {
	got := profile.DevelopmentProfile()
	if !containsAll(got.ToolIDs, profile.Git, profile.Docker, profile.Claude, profile.T3Code, profile.Node, profile.Bun) {
		t.Fatalf("development tools = %v", got.ToolIDs)
	}
	if containsAny(got.ToolIDs, profile.Mise, profile.UV, profile.OpenCode) {
		t.Fatalf("development profile unexpectedly includes optional tools: %v", got.ToolIDs)
	}
}

func TestResolveProfilesRejectsUnknownProfile(t *testing.T) {
	_, err := profile.ResolveProfiles([]string{"does-not-exist"})
	if err == nil {
		t.Fatal("ResolveProfiles() error = nil, want an unknown-profile error")
	}
}

func containsAll(ids []profile.ToolID, wanted ...profile.ToolID) bool {
	seen := make(map[profile.ToolID]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range wanted {
		if !seen[id] {
			return false
		}
	}
	return true
}

func containsAny(ids []profile.ToolID, wanted ...profile.ToolID) bool {
	for _, id := range ids {
		for _, candidate := range wanted {
			if id == candidate {
				return true
			}
		}
	}
	return false
}
