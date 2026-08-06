# Keyboard Selection and Color Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an arrow-key multi-select with green `[✓]` and red `[✗]` markers, plus restrained semantic color across all human-facing `jb` output.

**Architecture:** Keep `install.SelectionUI` as the service boundary. Add Huh-backed interactive and adaptive selectors in `internal/render`, centralize Lip Gloss styles and terminal/color policy in a renderer, and inject that renderer into install and CLI presentation without changing planning or adapter behavior. Preserve the numbered selector for non-TTY streams and use a cancellation sentinel plus `Summary.Cancelled` to exit cleanly before mutation.

**Tech Stack:** Go 1.26.5, Cobra 1.10.2, Huh 2.0.3, Lip Gloss 2.0.5, `golang.org/x/term` 0.45.0, existing Go/Bash/PowerShell tests.

## Global Constraints

- Scope is the end-user `jb` binary; do not style `cmd/release` or bootstrap installers.
- Real TTY controls are Up/Down, Space, Enter, Escape, and Ctrl+C.
- Selected is `[✓]` with only `✓` green; deselected is `[✗]` with only `✗` red; the active cursor is cyan `❯`.
- Escape and Ctrl+C print `Cancelled — no changes made`, perform no host mutation, and exit successfully.
- Non-TTY input or output uses the existing numbered selector with no ANSI escapes.
- `--yes`, `--dry-run`, profiles, `--only`, dependency order, detection, installation, update, and verification semantics must not change.
- `NO_COLOR` disables color regardless of value; redirected, completion, and machine-readable output contain no ANSI escapes.
- Color is never the sole state indicator; every state retains a symbol or word.
- Every commit is pushed immediately to `origin/main`, matching the user's auto-push preference.

---

## File Structure

- Create `internal/render/theme.go` and `theme_test.go`: semantic palette, `NO_COLOR`, terminal detection, light/dark choice, Huh theme, and deterministic test constructors.
- Create `internal/render/interactive_select.go` and tests: Huh multi-select, exact keys/markers, ordering, and cancellation mapping.
- Create `internal/render/adaptive_select.go` and tests: TTY/fallback routing and recoverable-start fallback.
- Modify `internal/render/select.go` and tests: retain the numbered fallback and add shared selection errors.
- Create `internal/render/output.go` and tests: plan, result, cancellation, version, error, and help rendering.
- Modify `internal/render/table.go`: route version tables through the semantic renderer.
- Modify `internal/install/install.go` and tests: distinguish cancellation and render every result state.
- Modify `internal/cli/root.go`, `tools.go`, `version.go`, and tests: inject theme, adaptive selector, help, version, and errors.
- Modify `cmd/jb/main.go`; create `cmd/jb/main_test.go`: themed error presentation.
- Modify `README.md`, `site/index.html`, and `tests/publication.bash`: public controls, symbols, fallback, and `NO_COLOR`.
- Modify `go.mod` and `go.sum`: pinned terminal UI dependencies.

---

### Task 1: Semantic Theme and Color Policy

**Files:**
- Create: `internal/render/theme.go`
- Create: `internal/render/theme_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `type ColorMode uint8` with `ColorAuto`, `ColorAlways`, and `ColorNever`.
- Produces: `type ThemeOptions struct { Mode ColorMode; Dark bool; Env []string }`.
- Produces: `func NewTheme(ThemeOptions) Theme`.
- Produces: `func AutoTheme(in io.Reader, out io.Writer, env []string) Theme`.
- Produces: `Accent`, `Success`, `Danger`, `Warning`, `Muted`, `Important`, `SelectedMarker`, `UnselectedMarker`, and `HuhTheme`.

- [ ] **Step 1: Add failing theme tests**

Create tests that force color, disable color, set an empty `NO_COLOR`, and inspect exact stripped markers:

```go
func TestThemeSelectionMarkersColorOnlyTheSymbol(t *testing.T) {
    theme := render.NewTheme(render.ThemeOptions{Mode: render.ColorAlways, Dark: true})
    if got := stripANSI(theme.SelectedMarker()); got != "[✓]" {
        t.Fatalf("selected marker = %q", got)
    }
    if got := stripANSI(theme.UnselectedMarker()); got != "[✗]" {
        t.Fatalf("unselected marker = %q", got)
    }
}

func TestAutoThemeHonorsNoColorEvenForTerminal(t *testing.T) {
    theme := render.AutoThemeForTest(true, true, []string{"NO_COLOR="})
    if strings.Contains(theme.Success("installed"), "\x1b[") {
        t.Fatal("NO_COLOR output contains ANSI")
    }
}
```

Use package `render` for tests that need the detector seam; keep `AutoThemeForTest` unexported as `autoTheme(inputTTY, outputTTY bool, env []string)`.

- [ ] **Step 2: Verify the focused tests fail**

Run:

```powershell
go test ./internal/render -run 'TestTheme|TestAutoTheme'
```

Expected: compilation fails because the theme API does not exist.

- [ ] **Step 3: Pin dependencies**

Run:

```powershell
go get charm.land/huh/v2@v2.0.3 charm.land/lipgloss/v2@v2.0.5 golang.org/x/term@v0.45.0
go mod tidy
```

Confirm `go.mod` retains `go 1.26.5` and lists all three as direct requirements.

- [ ] **Step 4: Implement the semantic theme**

Use these exact adaptive colors:

```go
choose := lipgloss.LightDark(opts.Dark)
accent := choose(lipgloss.Color("#007C91"), lipgloss.Color("#5FD7FF"))
success := choose(lipgloss.Color("#168A45"), lipgloss.Color("#5FD787"))
danger := choose(lipgloss.Color("#C72C41"), lipgloss.Color("#FF5F6D"))
warning := choose(lipgloss.Color("#9A6700"), lipgloss.Color("#FFD75F"))
muted := choose(lipgloss.Color("#667085"), lipgloss.Color("#8A8A8A"))
```

`hasNoColor` checks variable presence with `strings.Cut(value, "=")`, not a non-empty value. `AutoTheme` enables color only when both streams are `*os.File`, both descriptors satisfy `term.IsTerminal`, and `NO_COLOR` is absent. Background-query failure defaults to dark and never fails a command.

Markers concatenate neutral brackets around a styled symbol:

```go
func (t Theme) SelectedMarker() string {
    return "[" + t.Success("✓") + "]"
}

func (t Theme) UnselectedMarker() string {
    return "[" + t.Danger("✗") + "]"
}
```

Build `HuhTheme` from `huh.ThemeBase(isDark)`. Set `Focused.MultiSelectSelector` to cyan `❯ `, prefix strings to the two marker methods plus one space, title to bold cyan, help/description to muted, and errors to red. Do not set a foreground on the entire prefix.

- [ ] **Step 5: Verify and commit**

Run:

```powershell
gofmt -w internal/render/theme.go internal/render/theme_test.go
go test ./internal/render
go test ./...
git diff --check
git add go.mod go.sum internal/render/theme.go internal/render/theme_test.go
git commit -m "feat: add semantic terminal theme"
git push origin main
```

Expected: all checks pass and plain renderer snapshots remain unchanged.

---

### Task 2: Keyboard and Adaptive Selection

**Files:**
- Create: `internal/render/interactive_select.go`
- Create: `internal/render/interactive_select_test.go`
- Create: `internal/render/adaptive_select.go`
- Create: `internal/render/adaptive_select_test.go`
- Modify: `internal/render/select.go`
- Modify: `internal/render/select_test.go`

**Interfaces:**
- Consumes: `Theme.HuhTheme()` and existing `SelectionUI` / `Item`.
- Produces: `var ErrCancelled` and `var ErrInteractiveUnavailable`.
- Produces: `func NewInteractiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI`.
- Produces: `func NewAdaptiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI`.
- Produces internal `newAdaptiveSelection(terminal bool, interactive, fallback SelectionUI) SelectionUI` for deterministic tests.

- [ ] **Step 1: Add failing keyboard tests**

Feed Bubble Tea key bytes through Huh's injected input and output:

```go
func TestInteractiveSelectionMovesTogglesAndAccepts(t *testing.T) {
    input := bytes.NewBufferString("\x1b[B \r") // Down, Space, Enter.
    var output bytes.Buffer
    selection := render.NewInteractiveSelection(input, &output,
        render.NewTheme(render.ThemeOptions{Mode: render.ColorNever, Dark: true}))
    items := []render.Item{
        {Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git", Selected: true},
        {Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
    }

    got, err := selection.Select(context.Background(), items)
    if err != nil {
        t.Fatal(err)
    }
    if want := []tools.ToolID{profile.Git}; !reflect.DeepEqual(got, want) {
        t.Fatalf("selected = %v, want %v", got, want)
    }
}
```

Also test all-default acceptance, original-order return after toggles, Escape, Ctrl+C, exact ANSI-stripped `❯ [✓]` / `[✗]`, forced-color marker/cursor styles, and ANSI bold on the active tool label only. Use bounded contexts to prevent hangs. Run success, cancellation, and injected-runner-error cases sequentially and assert a second selector can start after each one; this guards terminal cleanup at the repository boundary while Huh retains ownership of raw-mode restoration.

- [ ] **Step 2: Add failing adaptive tests**

Use selection stubs to prove terminal streams call interactive, either non-terminal stream calls numbered fallback, and only `ErrInteractiveUnavailable` retries through fallback:

```go
func TestAdaptiveSelectionFallsBackWhenInteractiveIsUnavailable(t *testing.T) {
    interactive := &selectionStub{err: render.ErrInteractiveUnavailable}
    fallback := &selectionStub{ids: []tools.ToolID{profile.Git}}
    selection := newAdaptiveSelection(true, interactive, fallback)

    got, err := selection.Select(context.Background(), oneGitItem())
    if err != nil || !reflect.DeepEqual(got, []tools.ToolID{profile.Git}) {
        t.Fatalf("Select() = %v, %v", got, err)
    }
}
```

Assert the numbered fallback contains no `\x1b[`.

- [ ] **Step 3: Verify focused tests fail**

```powershell
go test ./internal/render -run 'TestInteractiveSelection|TestAdaptiveSelection|TestNumberedSelection'
```

Expected: compilation fails for the new constructors and sentinels.

- [ ] **Step 4: Implement the Huh selector**

Build one `huh.MultiSelect[tools.ToolID]` with options in item order and `.Selected(item.Selected)`. Disable filtering. Use title `Select tools` and description `↑/↓ move   Space toggle   Enter continue   Esc cancel`.

Use exact bindings:

```go
keys := huh.NewDefaultKeyMap()
keys.Quit = key.NewBinding(
    key.WithKeys("esc", "ctrl+c"),
    key.WithHelp("esc", "cancel"),
)
keys.MultiSelect.Toggle = key.NewBinding(
    key.WithKeys("space"),
    key.WithHelp("space", "toggle"),
)
```

Run `huh.NewForm(huh.NewGroup(field))` with `WithInput`, `WithOutput`, `WithTheme`, `WithKeyMap`, `WithShowHelp(false)`, and `RunWithContext(ctx)`. Map `huh.ErrUserAborted` to `ErrCancelled`; wrap all other errors as `interactive selection: %w`. Return IDs by iterating original items against a selected-ID set. Let Huh/Bubble Tea own terminal restoration.

Huh exposes the hovered value but not a cursor-specific option style. Wrap the multi-select in a small `interactiveField` that embeds `*huh.MultiSelect[tools.ToolID]`, forwards `Update` while retaining the wrapper, and overrides `View` to bold the hovered tool label after the Huh field renders it. Match the unique tool ID/label pair, so only the active row receives ANSI bold; do not fork Huh's selection state or key handling.

- [ ] **Step 5: Implement adaptive routing**

Use numbered selection immediately when either stream is not a terminal. On terminals, invoke interactive once. Fall back only for `ErrInteractiveUnavailable`; cancellation and I/O failures return unchanged so consumed input is never replayed.

- [ ] **Step 6: Verify, commit, and push**

```powershell
gofmt -w internal/render/select.go internal/render/select_test.go internal/render/interactive_select.go internal/render/interactive_select_test.go internal/render/adaptive_select.go internal/render/adaptive_select_test.go
go test ./internal/render
go test ./...
go vet ./...
git diff --check
git add internal/render/select.go internal/render/select_test.go internal/render/interactive_select.go internal/render/interactive_select_test.go internal/render/adaptive_select.go internal/render/adaptive_select_test.go
git commit -m "feat: add keyboard tool selection"
git push origin main
```

---

### Task 3: Cancellation and Selector Integration

**Files:**
- Modify: `internal/install/install.go`
- Modify: `internal/install/install_test.go`
- Modify: `internal/cli/tools.go`
- Modify: `internal/cli/tools_test.go`

**Interfaces:**
- Consumes: `render.ErrCancelled`, `render.NewAdaptiveSelection`, and `render.Theme`.
- Produces: `Summary.Cancelled bool`.
- Produces internal `type selectionFactory func(in io.Reader, out io.Writer, theme render.Theme) install.SelectionUI`.

- [ ] **Step 1: Add failing cancellation tests**

```go
func TestRunTreatsSelectionCancellationAsSuccessfulNoOp(t *testing.T) {
    adapter := &fixtureAdapter{}
    selection := &fixtureSelection{err: render.ErrCancelled}
    summary := install.Run(context.Background(), install.Install,
        []install.ToolStatus{{Tool: mustTool(t, profile.Git), Selected: true}},
        fixtureAdapters(adapter), install.Options{Writer: io.Discard, Selection: selection})

    if summary.Failed || !summary.Cancelled || len(adapter.calls) != 0 {
        t.Fatalf("summary = %#v, calls = %v", summary, adapter.calls)
    }
}
```

Add a second test proving an ordinary selection error is still failed and contextualized.

- [ ] **Step 2: Add failing CLI integration tests**

Inject a selector factory. Prove the command requests adaptive selection from its actual input/output, `--yes` still produces zero selection calls, and cancellation returns nil with zero adapter mutation.

- [ ] **Step 3: Verify red**

```powershell
go test ./internal/install ./internal/cli -run 'Cancellation|AdaptiveSelection|YesSkips'
```

Expected: failure because `Summary.Cancelled` and the factory do not exist.

- [ ] **Step 4: Implement cancellation**

Extend:

```go
type Summary struct {
    Results   []ToolResult
    Failed    bool
    Cancelled bool
}
```

Immediately after `Selection.Select`, detect `errors.Is(err, render.ErrCancelled)` before generic errors and return `Summary{Cancelled: true}`. Do not print `selection failed`, run dependency ordering, or call an adapter.

- [ ] **Step 5: Wire adaptive selection**

Replace `render.NewNumberedSelection` in the tools command with the injected factory, defaulting to `render.NewAdaptiveSelection`. Build the theme from the same command input/output and `os.Environ()`. Keep command tests deterministic using an injected factory and `ColorNever`. `toolsService.Run` returns nil for cancellation.

- [ ] **Step 6: Verify, commit, and push**

```powershell
gofmt -w internal/install/install.go internal/install/install_test.go internal/cli/tools.go internal/cli/tools_test.go
go test ./internal/install ./internal/cli
go test ./...
go vet ./...
git diff --check
git add internal/install/install.go internal/install/install_test.go internal/cli/tools.go internal/cli/tools_test.go
git commit -m "feat: integrate cancellable tool selector"
git push origin main
```

---

### Task 4: Semantic Plans, Results, Errors, Version, and Help

**Files:**
- Create: `internal/render/output.go`
- Create: `internal/render/output_test.go`
- Modify: `internal/render/table.go`
- Modify: `internal/install/install.go`
- Modify: `internal/install/install_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Modify: `internal/cli/version.go`
- Modify: `cmd/jb/main.go`
- Create: `cmd/jb/main_test.go`

**Interfaces:**
- Produces: `type Renderer struct`, `NewRenderer(io.Writer, Theme) *Renderer`, and `NewPlainRenderer(io.Writer) *Renderer`.
- Produces: `type ResultRow struct { Action, Tool, Status string; Err error }`.
- Produces methods `VersionTable`, `Result`, `Cancelled`, `Version`, `Error`, and `Help`.
- Produces: `install.Options.Renderer *render.Renderer`.
- Produces: `func cli.PrintError(io.Writer, error) error`.

- [ ] **Step 1: Add failing output tests**

Test exact plain mappings and forced semantic color:

```go
func TestRendererResultUsesSemanticSymbolsAndWords(t *testing.T) {
    tests := []struct{ status, want string }{
        {"installed", "✓ installed Git"},
        {"updated", "✓ updated Git"},
        {"up-to-date", "✓ up-to-date Git"},
        {"dry-run", "! install Git: dry-run"},
        {"skipped", "- skipped Git"},
        {"failed", "✗ install Git failed"},
    }
    for _, test := range tests {
        var output bytes.Buffer
        r := render.NewPlainRenderer(&output)
        if err := r.Result(render.ResultRow{Action: "install", Tool: "Git", Status: test.status}); err != nil {
            t.Fatal(err)
        }
        if got := strings.TrimSpace(output.String()); got != test.want {
            t.Fatalf("%s = %q, want %q", test.status, got, test.want)
        }
    }
}
```

Add writer-failure tests for every public method. In forced-color mode assert cyan headings, green success, yellow dry-run, red failure, and muted skipped content; after stripping ANSI, text must match plain mode.

- [ ] **Step 2: Add failing CLI presentation tests**

Prove:

- redirected version remains exactly `Johan Bostrom CLI dev\n`;
- forced-color version contains ANSI but strips to the same text;
- forced-color help styles headings, commands, and flags;
- redirected help has no ANSI;
- `completion powershell` has no ANSI even when human color is forced;
- `PrintError` renders `✗ error: boom` for `errors.New("boom")` and preserves writer failures.

- [ ] **Step 3: Verify red**

```powershell
go test ./internal/render ./internal/cli ./cmd/jb -run 'Renderer|Version|Help|Completion|PrintError'
```

Expected: compilation fails for `Renderer`, `ResultRow`, and `PrintError`.

- [ ] **Step 4: Implement the renderer**

Each method builds one semantic line and performs one checked write. Never wrap stdout globally. Keep tabwriter tabs/padding unstyled.

Map states exactly:

```go
switch row.Status {
case "installed", "updated", "up-to-date":
    line = theme.Success("✓") + " " + theme.Success(row.Status) + " " + theme.Important(row.Tool)
case "dry-run":
    line = theme.Warning("!") + " " + theme.Warning(row.Action) + " " + theme.Important(row.Tool) + ": " + theme.Warning("dry-run")
case "skipped":
    line = theme.Muted("- skipped " + row.Tool)
case "failed":
    line = theme.Danger("✗") + " " + theme.Danger(row.Action+" "+row.Tool+" failed")
}
```

Append the concrete error text after failed/skipped rows. `Cancelled()` writes the exact approved sentence. `Error(errors.New("boom"))` writes `✗ error: boom`.

- [ ] **Step 5: Render install plans and every result**

Add `Renderer *render.Renderer` to `install.Options`. If nil, construct a plain renderer from `Writer`. Replace direct table and dry-run writes. Render every installed, updated, up-to-date, skipped, failed, and dry-run result once.

On cancellation, call `Renderer.Cancelled()`. If that write fails, return a failed summary but keep adapter calls empty. If result rendering fails after mutation, stop subsequent execution, preserve earlier results, add the rendering failure, and return `Failed: true`.

- [ ] **Step 6: Style version, errors, and help**

Create a renderer from command streams/environment. Version calls `Renderer.Version("Johan Bostrom CLI", version.Version)`.

Install a custom Cobra help function that preserves ordering and completion behavior while styling human help tokens only. Never route generated completion content through `Renderer`.

Add:

```go
func PrintError(writer io.Writer, err error) error {
    theme := render.AutoTheme(os.Stdin, writer, os.Environ())
    return render.NewRenderer(writer, theme).Error(err)
}
```

Use an unexported theme-injected seam in tests. `cmd/jb/main.go` calls `cli.PrintError` and falls back to `fmt.Fprintln` only if rendering fails.

- [ ] **Step 7: Verify, commit, and push**

```powershell
gofmt -w internal/render/output.go internal/render/output_test.go internal/render/table.go internal/install/install.go internal/install/install_test.go internal/cli/root.go internal/cli/root_test.go internal/cli/version.go cmd/jb/main.go cmd/jb/main_test.go
go test ./internal/render ./internal/install ./internal/cli ./cmd/jb
go test ./...
go vet ./...
go build ./cmd/jb ./cmd/release
git diff --check
git add internal/render/output.go internal/render/output_test.go internal/render/table.go internal/install/install.go internal/install/install_test.go internal/cli/root.go internal/cli/root_test.go internal/cli/version.go cmd/jb/main.go cmd/jb/main_test.go
git commit -m "feat: colorize jb terminal output"
git push origin main
```

---

### Task 5: Documentation and Cross-Platform Verification

**Files:**
- Modify: `README.md`
- Modify: `site/index.html`
- Modify: `tests/publication.bash`

**Interfaces:**
- Consumes the final controls, symbols, fallback, `NO_COLOR`, and unchanged `--yes`.
- Produces public documentation only.

- [ ] **Step 1: Add failing publication assertions**

Add matching README and site assertions:

```bash
assert_contains "$readme" "↑/↓" "README documents arrow-key navigation"
assert_contains "$readme" "Space" "README documents selection toggling"
assert_contains "$readme" "[✓]" "README shows selected tools"
assert_contains "$readme" "[✗]" "README shows deselected tools"
assert_contains "$readme" "NO_COLOR" "README documents color opt-out"
assert_contains "$readme" "numbered" "README documents redirected-input fallback"
```

Preserve assertions for plain install, `--yes`, profiles, and `--only`.

- [ ] **Step 2: Verify publication tests fail**

```powershell
& 'C:\Program Files\Git\bin\bash.exe' tests/publication.bash
```

Expected: all new keyboard/color assertions fail.

- [ ] **Step 3: Update README and site**

Show:

```text
❯ [✓] Git
  [✓] GitHub CLI
  [✗] Docker
```

Explain Up/Down, Space, Enter, Escape/Ctrl+C, all-selected defaults, numbered redirected fallback, `--yes`, and `NO_COLOR`. Do not imply color is required to understand state.

- [ ] **Step 4: Run automated verification**

```powershell
& 'C:\Program Files\Git\bin\bash.exe' tests/publication.bash
go test -count=1 ./...
go vet ./...
go build ./cmd/jb ./cmd/release
& 'C:\Program Files\Git\bin\bash.exe' tests/run.bash
pwsh -NoProfile -File tests/cli-smoke.ps1
git diff --check
```

Expected: all exit zero and only Task 5 files remain modified.

- [ ] **Step 5: Run a real-terminal smoke test**

Run `go run ./cmd/jb tools install --dry-run` in Windows Terminal or a native Linux terminal. Verify cursor movement, toggling, Enter results, Escape cancellation, terminal restoration, and readable light/dark output. Then set `NO_COLOR=1` (PowerShell: `$env:NO_COLOR='1'`) and verify symbols remain without color; remove it afterward with `Remove-Item Env:NO_COLOR`.

- [ ] **Step 6: Commit and push**

```powershell
git add README.md site/index.html tests/publication.bash
git commit -m "docs: explain keyboard tool selection"
git push origin main
```

---

### Task 6: Independent Review and Final Verification

**Files:**
- Review all changes from this plan commit through Task 5 HEAD.
- Modify only files required by validated findings.

**Interfaces:**
- Consumes the approved design and complete implementation.
- Produces reviewed, verified, synchronized `main`.

- [ ] **Step 1: Request read-only review**

Use `superpowers:requesting-code-review` with:

- Design: `docs/superpowers/specs/2026-08-06-keyboard-selection-and-color-design.md`
- Plan: `docs/superpowers/plans/2026-08-06-keyboard-selection-and-color.md`
- Base SHA: this plan commit
- Head SHA: Task 5 commit
- Review points: terminal restoration, cancellation before mutation, key behavior, marker styling, `NO_COLOR`, redirected/completion ANSI safety, write failures, Windows/Linux compatibility, and unchanged release tooling.

- [ ] **Step 2: Resolve findings test-first**

For every Critical or Important finding, add a regression test, run it red, implement the smallest fix, and run focused tests green. Re-request review after any architectural change. Fix low-risk Minor findings or record them for later.

- [ ] **Step 3: Commit and push fixes if needed**

```powershell
git add go.mod go.sum internal/render internal/install/install.go internal/install/install_test.go internal/cli/root.go internal/cli/root_test.go internal/cli/tools.go internal/cli/tools_test.go internal/cli/version.go cmd/jb/main.go cmd/jb/main_test.go README.md site/index.html tests/publication.bash
git commit -m "fix: address terminal UI review"
git push origin main
```

Do not create an empty commit.

- [ ] **Step 4: Run fresh final verification**

```powershell
go test -count=1 ./...
go vet ./...
go build ./cmd/jb ./cmd/release
& 'C:\Program Files\Git\bin\bash.exe' tests/run.bash
pwsh -NoProfile -File tests/cli-smoke.ps1
git diff --check
git status --short
git rev-parse HEAD
git rev-parse origin/main
```

Expected: all checks pass, status is empty, and HEAD equals `origin/main`.

- [ ] **Step 5: Finish**

Use `superpowers:verification-before-completion`, then `superpowers:finishing-a-development-branch`. Because execution commits directly to synchronized `main`, report the final pushed commit rather than creating a redundant merge or pull request.
