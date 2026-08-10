package skills

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCatalogInstallAndUpdateOnlyChecksAvailableEntries(t *testing.T) {
	environment := testEnvironment(t)
	firstSource := filepath.Join(environment.WorkDir, "sources", "review-skill")
	secondSource := filepath.Join(environment.WorkDir, "sources", "summarize-skill")
	writeSkill(t, firstSource, "Review code.")
	writeSkill(t, secondSource, "Summarize changes.")
	entries := []CatalogEntry{
		{ID: "review-skill", Name: "Review", Description: "Review code", Source: firstSource},
		{ID: "summarize-skill", Name: "Summarize", Description: "Summarize changes", Source: secondSource},
	}
	manager, err := NewManagerWithCatalog(environment, entries)
	if err != nil {
		t.Fatal(err)
	}

	progress := []int{}
	statuses, err := manager.CheckCatalog(context.Background(), manager.Available(), false, func(completed, total int) error {
		progress = append(progress, completed)
		if total != 2 {
			t.Fatalf("progress total = %d, want 2", total)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress, []int{1, 2}) {
		t.Fatalf("progress = %v, want one callback per catalog entry", progress)
	}
	if statuses[0].Installed || statuses[1].Installed {
		t.Fatalf("initial statuses = %#v, want both not installed", statuses)
	}

	results, err := manager.InstallCatalog(context.Background(), statuses, []SkillID{"review-skill"}, CatalogOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "Review" || results[0].Status != "installed" {
		t.Fatalf("install results = %#v, want selected catalog entry", results)
	}

	writeSkill(t, firstSource, "Review code and verify the result.")
	statuses, err = manager.CheckCatalog(context.Background(), manager.Available(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !statuses[0].UpdateAvailable || statuses[1].UpdateAvailable {
		t.Fatalf("update statuses = %#v, want only review-skill updateable", statuses)
	}

	results, err = manager.UpdateCatalog(context.Background(), statuses, []SkillID{"review-skill"}, CatalogOperationOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "dry-run" || results[0].Name != "Review" {
		t.Fatalf("dry-run update results = %#v, want one selected update", results)
	}
}

func TestResolveCatalogRequiresExplicitAvailableSkill(t *testing.T) {
	entries := []CatalogEntry{{ID: "review-skill", Source: "./review-skill"}}
	resolved, err := ResolveCatalog(entries, []SkillID{"review-skill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].ID != "review-skill" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if _, err := ResolveCatalog(entries, []SkillID{"not-available"}); err == nil {
		t.Fatal("unknown catalog skill error = nil")
	}
	if _, err := ResolveCatalog(nil, nil); err != nil {
		t.Fatalf("empty catalog error = %v, want nil", err)
	}
}

func TestCatalogIncludesCreatorGroupsAndImpeccable(t *testing.T) {
	entries, err := ResolveCatalog(Catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 30 {
		t.Fatalf("catalog entries = %d, want 30", len(entries))
	}
	creators := make(map[string]int)
	for _, entry := range entries {
		creators[entry.Creator]++
	}
	if creators["Matt Pocock"] != 29 || creators["Paul Bakaus"] != 1 {
		t.Fatalf("creator groups = %#v, want Matt Pocock=29 and Paul Bakaus=1", creators)
	}
	impeccable, err := ResolveCatalog(Catalog, []SkillID{"impeccable"})
	if err != nil {
		t.Fatal(err)
	}
	if len(impeccable) != 1 || impeccable[0].Creator != "Paul Bakaus" {
		t.Fatalf("impeccable entry = %#v, want Paul Bakaus creator group", impeccable)
	}
}
