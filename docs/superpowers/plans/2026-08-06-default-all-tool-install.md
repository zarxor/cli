# Default-All Tool Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `jb tools install` plan the complete supported catalog by default while retaining interactive selection and all existing scoped forms.

**Architecture:** Remove the command-layer requirement for `--profiles` or `--only`, then make the existing service-level `requestedTools` function return a defensive catalog copy whenever no scope is supplied. Existing planning, selection, install/update convergence, and adapter behavior remain unchanged.

**Tech Stack:** Go 1.26.5, Cobra, existing fixture adapters and selection UI, Bash publication tests, README and static site documentation.

## Global Constraints

- `jb tools install` defaults to every entry in `tools.Catalog` in catalog order.
- Default interactive selection remains enabled and every eligible tool starts preselected.
- `jb tools install --yes` accepts the complete eligible plan without interaction.
- `--profiles`, `--only`, their combined form, `--dry-run`, and explicit validation errors retain current behavior.
- Already-installed tools retain current update/up-to-date convergence behavior.
- No `--all` flag, “all” profile, catalog change, or update-command behavior change is introduced.
- Every implementation commit is pushed to `origin` immediately after creation.

---

### Task 1: Default unqualified installs to the complete catalog

**Files:**
- Modify: `internal/cli/tools.go`
- Modify: `internal/cli/tools_test.go`

**Interfaces:**
- Consumes: `tools.Catalog`, `ToolsRequest`, `profile.ResolveProfiles`, `plan.MergeProfiles`, and `tools.ResolveTools`.
- Produces: unchanged `requestedTools(action install.Action, profiles []profile.Profile, only []tools.ToolID) ([]tools.Tool, error)` signature with a new empty-scope default.

- [ ] **Step 1: Replace the obsolete rejection test with a failing pass-through test**

Replace `TestToolsInstallRequiresProfilesOrOnly` with:

```go
func TestToolsInstallWithoutScopeReachesService(t *testing.T) {
	service := &recordingToolsService{}
	err := executeRoot(t, service, "tools", "install", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("service requests = %d, want one", len(service.requests))
	}
	request := service.requests[0]
	if len(request.ProfileNames) != 0 || len(request.Only) != 0 {
		t.Fatalf("scope = profiles %v, only %v; want empty", request.ProfileNames, request.Only)
	}
}
```

- [ ] **Step 2: Add failing full-catalog resolution tests**

```go
func TestToolsInstallWithoutScopePlansFullCatalog(t *testing.T) {
	adapter := newFixtureAdapter()
	err := executeRoot(t, fixtureService(adapter), "tools", "install", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]tools.ToolID, len(tools.Catalog))
	for index, tool := range tools.Catalog {
		want[index] = tool.ID
	}
	if !reflect.DeepEqual(adapter.detected, want) {
		t.Fatalf("detected tool IDs = %v, want full catalog %v", adapter.detected, want)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("dry-run adapter mutations = %v, want none", adapter.calls)
	}
}

func TestRequestedToolsReturnsDefensiveCatalogCopy(t *testing.T) {
	planned, err := requestedTools(install.Install, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(planned, tools.Catalog) {
		t.Fatalf("planned tools = %v, want catalog %v", planned, tools.Catalog)
	}
	planned[0].Name = "changed"
	if tools.Catalog[0].Name == "changed" {
		t.Fatal("requestedTools returned the mutable catalog slice")
	}
}
```

- [ ] **Step 3: Run the focused tests and verify the intended failures**

Run: `go test ./internal/cli -run 'TestToolsInstallWithoutScope|TestRequestedToolsReturnsDefensiveCatalogCopy'`

Expected: FAIL with `tools install requires --profiles or --only` and `tool selection is empty` because the new behavior is absent.

- [ ] **Step 4: Remove the command-layer selection requirement**

Change construction to:

```go
newToolsActionCommand(service, install.Install)
newToolsActionCommand(service, install.Update)
```

Change the function signature to:

```go
func newToolsActionCommand(service ToolsService, action install.Action) *cobra.Command
```

Delete the `requiresSelection` argument and the block returning `tools install requires --profiles or --only`. Continue passing empty `ProfileNames` and `Only` slices to the service.

- [ ] **Step 5: Add the service-level default before action-specific scoped behavior**

Implement:

```go
func requestedTools(action install.Action, profiles []profile.Profile, only []tools.ToolID) ([]tools.Tool, error) {
	if len(profiles) == 0 && len(only) == 0 {
		return append([]tools.Tool(nil), tools.Catalog...), nil
	}
	if action == install.Update && len(profiles) == 0 {
		return tools.ResolveTools(only)
	}
	return plan.MergeProfiles(profiles, only)
}
```

This preserves dependency expansion for install `--only` and existing update narrowing.

- [ ] **Step 6: Prove the complete catalog reaches the selection UI preselected**

Add this test-only selector:

```go
type recordingSelection struct {
	items []install.Item
}

func (s *recordingSelection) Select(_ context.Context, items []install.Item) ([]tools.ToolID, error) {
	s.items = append([]install.Item(nil), items...)
	return nil, nil
}
```

Add this test:

```go
func TestToolsInstallWithoutScopePreselectsFullCatalog(t *testing.T) {
	adapter := newFixtureAdapter()
	selection := &recordingSelection{}
	err := fixtureService(adapter).Run(context.Background(), ToolsRequest{
		Action: install.Install, Writer: io.Discard, Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.items) != len(tools.Catalog) {
		t.Fatalf("selection items = %d, want %d", len(selection.items), len(tools.Catalog))
	}
	for index, item := range selection.items {
		if item.Tool.ID != tools.Catalog[index].ID || !item.Selected {
			t.Fatalf("selection item %d = %#v, want preselected %s", index, item, tools.Catalog[index].ID)
		}
	}
}
```

- [ ] **Step 7: Run focused and complete Go verification**

Run: `gofmt -w internal/cli/tools.go internal/cli/tools_test.go`

Run: `go test ./internal/cli`

Run: `go test ./...`

Run: `go vet ./...`

Expected: every command exits 0. Existing scoped, invalid-input, update, and convergence tests remain green.

- [ ] **Step 8: Commit and push the behavior**

```text
git add internal/cli/tools.go internal/cli/tools_test.go
git commit -m "feat: install all tools by default"
git push origin main
```

---

### Task 2: Publish the new default in documentation

**Files:**
- Modify: `README.md`
- Modify: `site/index.html`
- Modify: `tests/publication.bash`

**Interfaces:**
- Consumes: completed Task 1 command behavior.
- Produces: public documentation for the default interactive and non-interactive forms.

- [ ] **Step 1: Add failing publication assertions**

Add:

```bash
assert_contains "$readme" 'jb tools install' "README documents default full-catalog installation"
assert_contains "$readme" 'jb tools install --yes' "README documents non-interactive full-catalog installation"
assert_contains "$readme" 'preselected' "README explains default interactive selection"

assert_contains "$site_page" '<code>jb tools install</code>' "site page documents default full-catalog installation"
assert_contains "$site_page" '<code>jb tools install --yes</code>' "site page documents non-interactive full-catalog installation"
assert_contains "$site_page" 'preselected' "site page explains default interactive selection"
```

- [ ] **Step 2: Run the publication test and verify it fails for the new copy**

Run: `bash tests/publication.bash`

Expected: FAIL because the README and site do not yet document both default forms with preselection semantics.

- [ ] **Step 3: Update README usage**

Make `jb tools install` the primary full-toolchain example. Explain that every supported tool is preselected and Enter accepts the defaults. Add `jb tools install --yes` as the non-interactive equivalent. Retain development-profile, named-profile, and `--only` examples as narrower alternatives.

- [ ] **Step 4: Update the published site command cards**

Change the primary install card to exact `<code>jb tools install</code>` content and explain that all supported tools are preselected. Add or update a card with exact `<code>jb tools install --yes</code>` content. Preserve profile and narrowed-profile examples elsewhere.

- [ ] **Step 5: Run complete verification**

Run: `go test -count=1 ./...`

Run: `go vet ./...`

Run: `go build ./cmd/jb ./cmd/release`

Run: `bash tests/run.bash`

Run: `git diff --check`

On Windows run: `pwsh -NoProfile -File tests/cli-smoke.ps1`

Expected: every command exits 0 and status lists only the intended documentation/test files before commit.

- [ ] **Step 6: Commit and push documentation**

```text
git add README.md site/index.html tests/publication.bash
git commit -m "docs: explain default all-tools install"
git push origin main
```

- [ ] **Step 7: Request final code review**

Run `superpowers:requesting-code-review` against the two implementation commits and the approved specification. Fix Important findings with focused regression tests, rerun Step 5, commit each fix, and push each fix immediately.
