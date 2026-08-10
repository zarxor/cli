package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/skills"
)

func TestSkillsInstallParsesCatalogSelectionFlags(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "--only=pdf,docx", "--dry-run", "--yes"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("requests = %d, want one", len(service.requests))
	}
	got := service.requests[0]
	got.Input = nil
	got.Writer = nil
	got.Renderer = nil
	got.Selection = nil
	got.HarnessSelection = nil
	want := SkillsRequest{
		Action:       SkillsInstall,
		Only:         []skills.SkillID{"pdf", "docx"},
		ScopeMode:    skills.ScopeModeGlobal,
		ScopeSet:     false,
		Harnesses:    []skills.Target{skills.TargetCodex, skills.TargetClaude},
		HarnessesSet: false,
		DryRun:       true,
		Yes:          true,
		Writer:       nil,
		Renderer:     nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request = %#v, want %#v", got, want)
	}
}

func TestSkillsInstallRejectsUnknownScopeMode(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "--scope=workspace"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("install error = nil, want unknown scope mode error")
	}
	if len(service.requests) != 0 {
		t.Fatalf("requests = %d, want zero", len(service.requests))
	}
}

func TestSkillsInstallParsesChooseScope(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "--scope=choose", "--yes", "--dry-run"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("requests = %d, want one", len(service.requests))
	}
	if got := service.requests[0].ScopeMode; got != skills.ScopeModeChoose {
		t.Fatalf("scope mode = %q, want %q", got, skills.ScopeModeChoose)
	}
	if !service.requests[0].ScopeSet {
		t.Fatal("scope set = false, want explicit scope selection")
	}
}

func TestSkillsInstallParsesHarnesses(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "--harnesses=claude", "--yes", "--dry-run"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("requests = %d, want one", len(service.requests))
	}
	got := service.requests[0]
	if !reflect.DeepEqual(got.Harnesses, []skills.Target{skills.TargetClaude}) || !got.HarnessesSet {
		t.Fatalf("harness request = %#v, want explicit Claude harness", got)
	}
}

func TestSkillsInstallRejectsUnknownHarness(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "--harnesses=cursor"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("install error = nil, want unknown harness error")
	}
	if len(service.requests) != 0 {
		t.Fatalf("requests = %d, want zero", len(service.requests))
	}
}

func TestSkillsInstallRejectsPositionalSource(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "install", "github:owner/repo/skill@main"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("install error = nil, want positional-source error")
	}
	if len(service.requests) != 0 {
		t.Fatalf("requests = %d, want zero", len(service.requests))
	}
}

func TestSkillsRemoveRequiresConfirmation(t *testing.T) {
	service := &recordingSkillsService{}
	root := newRootCommandWithServices(&recordingToolsService{}, service, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
	})
	root.SetArgs([]string{"skills", "remove", "pdf"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("remove error = nil, want confirmation error")
	}
	if len(service.requests) != 0 {
		t.Fatalf("requests = %d, want zero", len(service.requests))
	}
}

func TestSkillsServiceUsesCatalogInstallAndUpdateFlow(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "project")
	config := filepath.Join(home, "config")
	codex := filepath.Join(home, "codex")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	firstSource := filepath.Join(work, "sources", "review-skill")
	secondSource := filepath.Join(work, "sources", "summarize-skill")
	writeCLITestSkill(t, firstSource, "Review code.")
	writeCLITestSkill(t, secondSource, "Summarize changes.")
	manager, err := skills.NewManagerWithCatalog(skills.Environment{
		HomeDir:   home,
		WorkDir:   work,
		ConfigDir: config,
		CodexHome: codex,
	}, []skills.CatalogEntry{
		{ID: "review-skill", Name: "Review", Description: "Review code", Source: firstSource},
		{ID: "summarize-skill", Name: "Summarize", Description: "Summarize changes", Source: secondSource},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := &skillsService{manager: manager}
	var output bytes.Buffer
	if err := service.Run(context.Background(), SkillsRequest{
		Action:   SkillsInstall,
		Yes:      true,
		Writer:   &output,
		Renderer: render.NewPlainRenderer(&output),
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Checking installed skills") || !strings.Contains(output.String(), "installed") || !strings.Contains(output.String(), "Review") {
		t.Fatalf("install output = %q, want progress and selected result", output.String())
	}

	writeCLITestSkill(t, firstSource, "Review code and verify the result.")
	output.Reset()
	if err := service.Run(context.Background(), SkillsRequest{
		Action:   SkillsUpdate,
		Yes:      true,
		Writer:   &output,
		Renderer: render.NewPlainRenderer(&output),
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "updated") || !strings.Contains(output.String(), "Review") {
		t.Fatalf("update output = %q, want changed catalog skill", output.String())
	}
	if strings.Contains(output.String(), "Summarize") {
		t.Fatalf("update output = %q, want non-updateable skill hidden", output.String())
	}
}

func TestSkillsServiceInstallsToProjectScope(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "project")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(work, "sources", "project-skill")
	writeCLITestSkill(t, source, "Project-only instructions.")
	manager, err := skills.NewManagerWithCatalog(skills.Environment{
		HomeDir:   home,
		WorkDir:   work,
		ConfigDir: filepath.Join(home, "config"),
		CodexHome: filepath.Join(home, "codex"),
	}, []skills.CatalogEntry{{ID: "project-skill", Name: "Project skill", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	service := &skillsService{manager: manager}
	var output bytes.Buffer
	if err := service.Run(context.Background(), SkillsRequest{
		Action:    SkillsInstall,
		ScopeMode: skills.ScopeModeProject,
		Yes:       true,
		Writer:    &output,
		Renderer:  render.NewPlainRenderer(&output),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".agents", "skills", "project-skill", "SKILL.md")); err != nil {
		t.Fatalf("project skill = %v, want installed skill in project scope", err)
	}
	if _, err := os.Stat(filepath.Join(home, "codex", "skills", "project-skill")); !os.IsNotExist(err) {
		t.Fatalf("global skill stat error = %v, want no global installation", err)
	}
}

func TestSkillsServiceInstallsOnlyMissingSelectedHarness(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "project")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(work, "sources", "shared-skill")
	writeCLITestSkill(t, source, "Shared instructions.")
	manager, err := skills.NewManagerWithCatalog(skills.Environment{
		HomeDir:   home,
		WorkDir:   work,
		ConfigDir: filepath.Join(home, "config"),
		CodexHome: filepath.Join(home, "codex"),
	}, []skills.CatalogEntry{{ID: "shared-skill", Name: "Shared skill", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	service := &skillsService{manager: manager}
	if err := service.Run(context.Background(), SkillsRequest{
		Action:       SkillsInstall,
		ScopeMode:    skills.ScopeModeGlobal,
		Harnesses:    []skills.Target{skills.TargetCodex},
		HarnessesSet: true,
		Yes:          true,
		Writer:       io.Discard,
		Renderer:     render.NewPlainRenderer(io.Discard),
	}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := service.Run(context.Background(), SkillsRequest{
		Action:       SkillsInstall,
		ScopeMode:    skills.ScopeModeGlobal,
		Harnesses:    []skills.Target{skills.TargetCodex, skills.TargetClaude},
		HarnessesSet: true,
		Yes:          true,
		Writer:       &output,
		Renderer:     render.NewPlainRenderer(&output),
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(output.String(), "• installed"); got != 1 {
		t.Fatalf("install output = %q, want one missing harness installation", output.String())
	}
	if !strings.Contains(output.String(), "claude/global") || strings.Contains(output.String(), "codex/global") {
		t.Fatalf("install output = %q, want only Claude installation result", output.String())
	}
}

func TestSkillsServiceChoosesScopeAndHarnessesBeforeSkills(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "project")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	firstSource := filepath.Join(work, "sources", "global-skill")
	secondSource := filepath.Join(work, "sources", "project-skill")
	writeCLITestSkill(t, firstSource, "Global instructions.")
	writeCLITestSkill(t, secondSource, "Project instructions.")
	manager, err := skills.NewManagerWithCatalog(skills.Environment{
		HomeDir:   home,
		WorkDir:   work,
		ConfigDir: filepath.Join(home, "config"),
		CodexHome: filepath.Join(home, "codex"),
	}, []skills.CatalogEntry{
		{ID: "global-skill", Name: "Global skill", Source: firstSource},
		{ID: "project-skill", Name: "Project skill", Source: secondSource},
	})
	if err != nil {
		t.Fatal(err)
	}
	steps := []string{}
	selection := &skillSelection{ids: []render.SelectionID{"global-skill", "project-skill"}, steps: &steps, name: "skills"}
	harnessSelection := &skillSelection{ids: []render.SelectionID{render.SelectionID(skills.TargetCodex), render.SelectionID(skills.TargetClaude)}, steps: &steps, name: "harnesses"}
	service := &skillsService{manager: manager}
	var output bytes.Buffer
	if err := service.Run(context.Background(), SkillsRequest{
		Action:           SkillsInstall,
		Input:            strings.NewReader("project\n"),
		Writer:           &output,
		Renderer:         render.NewPlainRenderer(&output),
		Selection:        selection,
		HarnessSelection: harnessSelection,
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"global-skill", "project-skill"} {
		for _, root := range []string{filepath.Join(work, ".agents", "skills"), filepath.Join(work, ".claude", "skills")} {
			if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
				t.Fatalf("project skill %s in %s = %v, want project installation", name, root, err)
			}
		}
	}
	for _, path := range []string{
		filepath.Join(home, "codex", "skills"),
		filepath.Join(home, ".claude", "skills"),
	} {
		if _, err := os.Stat(filepath.Join(path, "global-skill")); !os.IsNotExist(err) {
			t.Fatalf("global skill stat error for %s = %v, want no global installation", path, err)
		}
	}
	if !strings.Contains(output.String(), "Installation scope") || strings.Contains(output.String(), "Scope for") {
		t.Fatalf("choose output = %q, want one setup scope prompt", output.String())
	}
	if !reflect.DeepEqual(steps, []string{"harnesses", "skills"}) {
		t.Fatalf("selection steps = %v, want harnesses before skills", steps)
	}
}

func TestCatalogInstallItemsDisableInstalledSkills(t *testing.T) {
	items := catalogInstallItems([]skills.CatalogStatus{
		{Entry: skills.CatalogEntry{ID: "installed-skill", Name: "Installed skill", Target: skills.TargetCodex}, Installed: true},
		{Entry: skills.CatalogEntry{ID: "new-skill", Name: "New skill", Target: skills.TargetCodex}},
	})
	if len(items) != 2 {
		t.Fatalf("items = %d, want two", len(items))
	}
	if !items[0].Disabled || items[0].Selected {
		t.Fatalf("installed item = %#v, want disabled and unselected", items[0])
	}
	if items[1].Disabled || !items[1].Selected {
		t.Fatalf("new item = %#v, want enabled and selected", items[1])
	}
}

func TestCatalogInstallItemsKeepPartiallyInstalledSkillsSelectable(t *testing.T) {
	items := catalogInstallItems([]skills.CatalogStatus{
		{Entry: skills.CatalogEntry{ID: "shared-skill", Name: "Shared skill", Target: skills.TargetCodex}, Installed: true},
		{Entry: skills.CatalogEntry{ID: "shared-skill", Name: "Shared skill", Target: skills.TargetClaude}},
	})
	if len(items) != 1 {
		t.Fatalf("items = %d, want one grouped skill", len(items))
	}
	if items[0].Disabled || !items[0].Selected {
		t.Fatalf("partial item = %#v, want enabled and selected", items[0])
	}
	if !strings.Contains(items[0].Label, "installed in codex") || !strings.Contains(items[0].Label, "missing in claude") {
		t.Fatalf("partial label = %q, want both harness states", items[0].Label)
	}
}

func TestSkillsServiceUpdateChecksBothScopes(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "project")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(work, "sources", "shared-skill")
	writeCLITestSkill(t, source, "Initial instructions.")
	manager, err := skills.NewManagerWithCatalog(skills.Environment{
		HomeDir:   home,
		WorkDir:   work,
		ConfigDir: filepath.Join(home, "config"),
		CodexHome: filepath.Join(home, "codex"),
	}, []skills.CatalogEntry{{ID: "shared-skill", Name: "Shared skill", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	service := &skillsService{manager: manager}
	for _, mode := range []skills.ScopeMode{skills.ScopeModeGlobal, skills.ScopeModeProject} {
		if err := service.Run(context.Background(), SkillsRequest{
			Action:    SkillsInstall,
			ScopeMode: mode,
			Yes:       true,
			Writer:    io.Discard,
			Renderer:  render.NewPlainRenderer(io.Discard),
		}); err != nil {
			t.Fatal(err)
		}
	}
	writeCLITestSkill(t, source, "Updated instructions.")
	var output bytes.Buffer
	if err := service.Run(context.Background(), SkillsRequest{
		Action:   SkillsUpdate,
		Yes:      true,
		Writer:   &output,
		Renderer: render.NewPlainRenderer(&output),
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(output.String(), "updated"); got != 4 {
		t.Fatalf("update output = %q, want both scopes across both harnesses", output.String())
	}
}

type recordingSkillsService struct {
	requests []SkillsRequest
}

type skillSelection struct {
	ids   []render.SelectionID
	steps *[]string
	name  string
}

func (s *skillSelection) Select(context.Context, []render.Item) ([]render.SelectionID, error) {
	if s.steps != nil {
		*s.steps = append(*s.steps, s.name)
	}
	return s.ids, nil
}

func (s *recordingSkillsService) Run(_ context.Context, request SkillsRequest) error {
	s.requests = append(s.requests, request)
	return nil
}

func writeCLITestSkill(t *testing.T, root, instruction string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + filepath.Base(root) + "\ndescription: A test skill.\n---\n\n" + instruction + "\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
