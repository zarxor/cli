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
			profile.Codex,
			profile.Node,
			profile.Bun,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DevelopmentProfile() = %#v, want %#v", got, want)
	}
}

func TestResolveProfilesRejectsUnknownProfile(t *testing.T) {
	_, err := profile.ResolveProfiles([]string{"does-not-exist"})
	if err == nil {
		t.Fatal("ResolveProfiles() error = nil, want an unknown-profile error")
	}
}
