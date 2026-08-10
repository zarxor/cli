package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallListUpdateVerifyAndRemoveLocalSkill(t *testing.T) {
	environment := testEnvironment(t)
	manager, err := NewManager(environment)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(environment.WorkDir, "source", "review-skill")
	writeSkill(t, source, "Review code carefully.")

	results, err := manager.Install(context.Background(), source, InstallOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "installed" {
		t.Fatalf("install results = %#v, want one installed result", results)
	}

	infos, err := manager.List(InspectOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Name != "review-skill" || !infos[0].Managed || !infos[0].Valid {
		t.Fatalf("list results = %#v, want one managed valid skill", infos)
	}

	writeSkill(t, source, "Review code carefully and verify the result.")
	results, err = manager.Update(context.Background(), UpdateOptions{Target: TargetCodex, Scope: ScopeUser, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "available" {
		t.Fatalf("dry-run update results = %#v, want one available result", results)
	}
	results, err = manager.Update(context.Background(), UpdateOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "updated" {
		t.Fatalf("update results = %#v, want one updated result", results)
	}

	checks, err := manager.Verify(context.Background(), InspectOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Status != "valid" {
		t.Fatalf("verify results = %#v, want one valid result", checks)
	}

	results, err = manager.Remove(context.Background(), RemoveOptions{Target: TargetCodex, Scope: ScopeUser, Names: []string{"review-skill"}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "dry-run" {
		t.Fatalf("dry-run remove results = %#v, want one dry-run result", results)
	}
	results, err = manager.Remove(context.Background(), RemoveOptions{Target: TargetCodex, Scope: ScopeUser, Names: []string{"review-skill"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "removed" {
		t.Fatalf("remove results = %#v, want one removed result", results)
	}
	if _, err := os.Stat(filepath.Join(environment.CodexHome, "skills", "review-skill")); !os.IsNotExist(err) {
		t.Fatalf("removed skill still exists, stat error = %v", err)
	}
}

func TestInstallRejectsExistingUnmanagedSkillWithoutForce(t *testing.T) {
	environment := testEnvironment(t)
	manager, err := NewManager(environment)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(environment.WorkDir, "source", "review-skill")
	destination := filepath.Join(environment.CodexHome, "skills", "review-skill")
	writeSkill(t, source, "Source skill.")
	writeSkill(t, destination, "Existing skill.")

	_, err = manager.Install(context.Background(), source, InstallOptions{Target: TargetCodex, Scope: ScopeUser})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("install error = %v, want existing-destination error", err)
	}
}

func TestVerifyReportsModifiedAndUnmanagedSkills(t *testing.T) {
	environment := testEnvironment(t)
	manager, err := NewManager(environment)
	if err != nil {
		t.Fatal(err)
	}
	managedSource := filepath.Join(environment.WorkDir, "source", "managed-skill")
	unmanagedPath := filepath.Join(environment.CodexHome, "skills", "unmanaged-skill")
	writeSkill(t, managedSource, "Managed skill.")
	writeSkill(t, unmanagedPath, "Unmanaged skill.")
	if _, err := manager.Install(context.Background(), managedSource, InstallOptions{Target: TargetCodex, Scope: ScopeUser}); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(environment.CodexHome, "skills", "managed-skill"), "Changed after installation.")

	checks, err := manager.Verify(context.Background(), InspectOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, check := range checks {
		statuses[check.Name] = check.Status
	}
	if statuses["managed-skill"] != "modified" || statuses["unmanaged-skill"] != "unmanaged" {
		t.Fatalf("verify statuses = %#v, want modified and unmanaged", statuses)
	}
}

func TestListSkipsAgentManagedCollectionsWithoutSkillMetadata(t *testing.T) {
	environment := testEnvironment(t)
	manager, err := NewManager(environment)
	if err != nil {
		t.Fatal(err)
	}
	collection := filepath.Join(environment.CodexHome, "skills", ".system")
	if err := os.MkdirAll(collection, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collection, "README.md"), []byte("agent-managed collection\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	infos, err := manager.List(InspectOptions{Target: TargetCodex, Scope: ScopeUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatalf("list results = %#v, want no collection entry", infos)
	}
}

func TestParseGitHubSource(t *testing.T) {
	got, err := parseSource("github:anthropics/skills/pdf@main", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != sourceGitHub || got.Owner != "anthropics" || got.Repo != "skills" || got.Subpath != "pdf" || got.Ref != "main" {
		t.Fatalf("parsed source = %#v", got)
	}

	got, err = parseSource("https://github.com/anthropics/skills/tree/main/pdf", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != sourceGitHub || got.Subpath != "pdf" || got.Ref != "main" {
		t.Fatalf("parsed GitHub URL = %#v", got)
	}
}

func TestParseMetadataRequiresStandardFrontmatter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readMetadata(root); err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("metadata error = %v, want frontmatter error", err)
	}
}

func testEnvironment(t *testing.T) Environment {
	t.Helper()
	home := t.TempDir()
	work := filepath.Join(home, "project")
	config := filepath.Join(home, "config")
	codex := filepath.Join(home, "codex")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	return Environment{HomeDir: home, WorkDir: work, ConfigDir: config, CodexHome: codex}
}

func writeSkill(t *testing.T, root, instruction string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + filepath.Base(root) + "\ndescription: A test skill.\n---\n\n" + instruction + "\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
