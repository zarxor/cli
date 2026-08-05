package plan_test

import (
	"reflect"
	"testing"

	"github.com/zarxor/scripts/internal/plan"
	"github.com/zarxor/scripts/internal/profile"
)

func TestMergeProfilesExpandsDependenciesInCatalogOrder(t *testing.T) {
	got, err := plan.MergeProfiles([]profile.Profile{profile.DevelopmentProfile()}, nil)
	if err != nil {
		t.Fatalf("MergeProfiles() error = %v", err)
	}

	want := []profile.ToolID{
		profile.Git, profile.GitHubCLI, profile.Docker, profile.DockerBuildx,
		profile.DockerCompose, profile.Codex, profile.NVM, profile.Node,
		profile.NPM, profile.Corepack, profile.PNPM, profile.Yarn, profile.Bun,
	}
	if gotIDs := toolIDs(got); !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("MergeProfiles() IDs = %v, want %v", gotIDs, want)
	}
}

func TestMergeProfilesDeduplicatesSharedToolIDs(t *testing.T) {
	profiles := []profile.Profile{
		{Name: "first", ToolIDs: []profile.ToolID{profile.Git, profile.Docker}},
		{Name: "second", ToolIDs: []profile.ToolID{profile.Git, profile.Docker}},
	}
	got, err := plan.MergeProfiles(profiles, nil)
	if err != nil {
		t.Fatalf("MergeProfiles() error = %v", err)
	}

	want := []profile.ToolID{profile.Git, profile.Docker, profile.DockerBuildx, profile.DockerCompose}
	if gotIDs := toolIDs(got); !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("MergeProfiles() IDs = %v, want %v", gotIDs, want)
	}
}

func TestMergeProfilesOnlyIntersectsProfileSelection(t *testing.T) {
	got, err := plan.MergeProfiles([]profile.Profile{profile.DevelopmentProfile()}, []profile.ToolID{profile.Docker})
	if err != nil {
		t.Fatalf("MergeProfiles() error = %v", err)
	}

	want := []profile.ToolID{profile.Docker, profile.DockerBuildx, profile.DockerCompose}
	if gotIDs := toolIDs(got); !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("MergeProfiles() IDs = %v, want %v", gotIDs, want)
	}
}

func TestMergeProfilesAcceptsOnlyWithoutProfile(t *testing.T) {
	got, err := plan.MergeProfiles(nil, []profile.ToolID{profile.Git})
	if err != nil {
		t.Fatalf("MergeProfiles() error = %v", err)
	}

	if gotIDs := toolIDs(got); !reflect.DeepEqual(gotIDs, []profile.ToolID{profile.Git}) {
		t.Fatalf("MergeProfiles() IDs = %v, want [git]", gotIDs)
	}
}

func TestMergeProfilesRejectsEmptySelection(t *testing.T) {
	_, err := plan.MergeProfiles(nil, nil)
	if err == nil {
		t.Fatal("MergeProfiles() error = nil, want an empty-selection error")
	}
}

func TestDependencyOrderPlacesDependenciesBeforeDependents(t *testing.T) {
	selected := []profile.Tool{
		{ID: "dependent", Name: "Dependent", Dependencies: []profile.ToolID{"base"}},
		{ID: "independent", Name: "Independent"},
		{ID: "base", Name: "Base"},
		{ID: "base", Name: "Duplicate Base"},
	}

	got, err := plan.DependencyOrder(selected)
	if err != nil {
		t.Fatal(err)
	}
	want := []profile.ToolID{"base", "dependent", "independent"}
	if gotIDs := toolIDs(got); !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("DependencyOrder() IDs = %v, want %v", gotIDs, want)
	}
}

func TestDependencyOrderRejectsCycles(t *testing.T) {
	selected := []profile.Tool{
		{ID: "first", Dependencies: []profile.ToolID{"second"}},
		{ID: "second", Dependencies: []profile.ToolID{"first"}},
	}

	if _, err := plan.DependencyOrder(selected); err == nil {
		t.Fatal("DependencyOrder() error = nil, want cycle error")
	}
}

func toolIDs(got []profile.Tool) []profile.ToolID {
	ids := make([]profile.ToolID, len(got))
	for i, tool := range got {
		ids[i] = tool.ID
	}
	return ids
}
