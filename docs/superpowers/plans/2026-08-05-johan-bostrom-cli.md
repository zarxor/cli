# Johan Bostrom CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Linux-only setup script with the cross-platform `jb` Go CLI for installing and live-updating development tools on Debian-family Linux, Arch Linux, and Windows.

**Architecture:** A Cobra command tree calls a stateless planner. The planner merges built-in profiles into unique tool IDs, performs live detection, renders an interactive selection, and dispatches ordered actions through Debian/Arch/Windows adapters. GitHub Releases will carry binaries and GitHub Pages will host bootstrap installers and documentation; release workflows remain deferred while Actions capacity is unavailable.

**Tech Stack:** Go 1.24+, Cobra 1.10.2, Go standard library (`context`, `os/exec`, `crypto/sha256`, `runtime`, `net/http`), GitHub Releases, GitHub Pages, Bash, and PowerShell.

## Global Constraints

- Product name: **Johan Bostrom CLI**; installed command: `jb`.
- Public host: `https://cli.johanbostrom.se`.
- Initial platforms: Debian/Ubuntu, Arch Linux, and Windows; macOS is deferred but the adapter boundary must allow it later.
- Initial profile: `development`; profiles are built into the binary and never persisted.
- Multiple profiles use `--profiles=<name>[,<name>...]`; merged tools and dependencies are deduplicated by stable tool ID.
- `jb tools install` shows the merged tool list and permits deselection unless `--yes` is supplied.
- `jb tools update` discovers installed tools and versions live; missing tools are not shown.
- Current and candidate versions are displayed before an interactive update; installed tools are pre-selected.
- `--dry-run` always renders a plan and performs no mutations.
- No local state database, telemetry, selection history, or hidden network service.
- No backwards compatibility with `scripts.johanbostrom.se` or `linux/dev-server/setup.sh`.
- GitHub Actions release automation is deferred; local builds and checks must work without Actions.
- Linux supports root and non-root users; non-root system actions use `sudo` and root runs privileged actions directly.
- Windows requests elevation only for operations that require it.
- Existing installations converge in place; profile edits and package sources are idempotent.
- Commands execute through structured argument arrays, never interpolated shell strings.

---

## File map

Create the Go CLI under these focused boundaries:

```text
go.mod
go.sum
cmd/jb/main.go
internal/cli/root.go
internal/cli/tools.go
internal/cli/version.go
internal/version/version.go
internal/profile/profile.go
internal/profile/catalog.go
internal/plan/plan.go
internal/plan/plan_test.go
internal/runner/runner.go
internal/runner/exec.go
internal/runner/fixture.go
internal/detect/detect.go
internal/adapters/debian.go
internal/adapters/arch.go
internal/adapters/windows.go
internal/tools/catalog.go
internal/tools/tools_test.go
internal/install/install.go
internal/install/install_test.go
internal/render/select.go
internal/render/select_test.go
internal/render/table.go
internal/platform/platform.go
internal/platform/platform_test.go
install.sh
install.ps1
tests/cli-smoke.ps1
tests/cli-smoke.sh
```

The old `linux/dev-server/setup.sh`, its Bash tests, the
`scripts.johanbostrom.se` `CNAME`, and the old README instructions are removed
or replaced in the documentation migration task. Do not leave a second
installer implementation that can drift from `jb`.

## Task 1: Scaffold the Go module and root Cobra command

**Files:**

- Create: `go.mod`, `go.sum`
- Create: `cmd/jb/main.go`
- Create: `internal/cli/root.go`, `internal/cli/root_test.go`
- Create: `internal/cli/version.go`, `internal/version/version.go`

**Interfaces:**

- `cmd/jb/main.go` calls `cli.Execute(context.Background(), os.Args[1:])` and exits with the returned status.
- `internal/cli.Execute(ctx context.Context, args []string) error` constructs the root Cobra command and executes it.
- `internal/version.Version` is a build-injected string with a development fallback.

- [ ] **Step 1: Write the failing tests**

```go
func TestExecuteVersion(t *testing.T) {
    output := new(bytes.Buffer)
    err := ExecuteWithIO(context.Background(), []string{"version"}, output, io.Discard)
    if err != nil { t.Fatal(err) }
    if !strings.Contains(output.String(), "Johan Bostrom CLI") { t.Fatal(output.String()) }
}
```

Add a second test that `help` lists `tools` and `version`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli`

Expected: FAIL because the Go module and command implementation do not exist.

- [ ] **Step 3: Write the minimal implementation**

Run `go mod init github.com/zarxor/scripts` and
`go get github.com/spf13/cobra@v1.10.2`. Create the root command with `Use:
"jb"`, `Short: "Johan Bostrom CLI"`, and `version`/`help` commands. Keep
`ExecuteWithIO` injectable so tests never need to spawn a process.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add go.mod go.sum cmd/jb internal/cli internal/version
git commit -m "feat: scaffold jb cobra cli"
```

## Task 2: Define tool IDs, profiles, dependencies, and deduplication

**Files:**

- Create: `internal/profile/profile.go`, `internal/profile/catalog.go`
- Create: `internal/tools/catalog.go`, `internal/tools/tools_test.go`
- Create: `internal/plan/plan.go`, `internal/plan/plan_test.go`

**Interfaces:**

```go
type ToolID string
type ProfileName string

type Tool struct {
    ID           ToolID
    Name         string
    Dependencies []ToolID
}

type Profile struct {
    Name    ProfileName
    ToolIDs []ToolID
}

func DevelopmentProfile() Profile
func ResolveProfiles(names []string) ([]Profile, error)
func ResolveTools(ids []ToolID) ([]Tool, error)
func MergeProfiles(profiles []Profile, only []ToolID) ([]Tool, error)
```

`DevelopmentProfile` contains `git`, `github-cli`, `docker`, `codex`, `node`,
and `bun`. `node` expands to nvm, Node.js LTS, npm, Corepack, pnpm, and Yarn;
Docker expands to Buildx and Compose. `MergeProfiles` returns stable catalog
order, removes duplicate IDs, expands dependencies once, and errors on unknown
profiles or tools.

- [ ] **Step 1: Write failing tests**

Cover development profile contents; unknown profile rejection; two profiles
sharing Git and Docker; dependency expansion; `--only` intersection; stable
order; and an empty selection error.

Also cover explicit tool-only resolution so `jb tools install --only=git` can
run without a profile.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/profile ./internal/tools ./internal/plan`

Expected: FAIL because the types and resolver do not exist.

- [ ] **Step 3: Implement the catalog and planner**

Use constants for every ID and a map for catalog lookup. Deduplicate with a
`map[ToolID]struct{}` while emitting in catalog order. Never deduplicate by
display name.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/profile ./internal/tools ./internal/plan`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/profile internal/tools internal/plan
git commit -m "feat: add profiles and deduplicated tool planning"
```

## Task 3: Add platform detection, structured command execution, and fixtures

**Files:**

- Create: `internal/platform/platform.go`, `internal/platform/platform_test.go`
- Create: `internal/runner/runner.go`, `internal/runner/exec.go`, `internal/runner/fixture.go`
- Create: `internal/detect/detect.go`, `internal/detect/detect_test.go`

**Interfaces:**

```go
type OS string
const (Debian OS = "debian"; Arch OS = "arch"; Windows OS = "windows")

type Runner interface {
    LookPath(ctx context.Context, name string) (string, error)
    Run(ctx context.Context, command string, args ...string) (Result, error)
}

type Elevation interface {
    RunElevated(ctx context.Context, command string, args ...string) (Result, error)
}

type Result struct { Stdout, Stderr string; ExitCode int }
type Detection struct { Installed bool; Current string; Candidate string }
```

`exec.Runner` uses `exec.CommandContext` with an argument slice. The fixture
runner records exact commands and returns configured results, so adapter tests
never install packages on the host. Platform detection uses `runtime.GOOS`,
`/etc/os-release` for Linux family detection, and Windows runtime detection.

- [ ] **Step 1: Write failing tests**

Cover Debian vs Arch fixture files, Windows detection, unsupported Linux, exact
argument preservation, exit-code propagation, and live version parsing from
stdout/stderr.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/platform ./internal/runner ./internal/detect`

Expected: FAIL because the interfaces and implementations do not exist.

- [ ] **Step 3: Implement the runner and detection boundaries**

Keep command logging in the fixture runner and never construct a shell command
string. Return command errors with the executable and exit code included.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/platform ./internal/runner ./internal/detect`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/platform internal/runner internal/detect
git commit -m "feat: add platform detection and command runner"
```

## Task 4: Implement Debian and Arch tool adapters

**Files:**

- Create: `internal/adapters/debian.go`, `internal/adapters/arch.go`
- Create: `internal/adapters/linux_test.go`

**Interfaces:**

- Import `internal/tools`, `internal/detect`, and `internal/runner` in the
  adapter package.

```go
type Adapter interface {
    Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error)
    Install(ctx context.Context, tool tools.Tool) error
    Update(ctx context.Context, tool tools.Tool) error
    Verify(ctx context.Context, tool tools.Tool) error
}
```

Use `detect.Detection` from `internal/detect` and implement the `Adapter`
interface using the fixture `Runner` first. Debian/
Ubuntu uses apt and supported GitHub CLI/Docker repositories; Arch uses pacman
transactions. Root invokes commands directly; non-root prefixes only system
operations with `sudo`. User-level tools use the invoking home directory.

Preserve the existing safety decisions: remove Docker package conflicts only
when installed, preserve `/var/lib/docker`, converge repository definitions,
and install Bun’s `unzip` prerequisite. Codex, nvm, Bun, and Node ecosystem
installers must clean temporary files and propagate installer exit codes.

- [ ] **Step 1: Write failing adapter tests**

Use fixtures to assert Debian and Arch install/update command plans, root vs
non-root privilege prefixes, Docker conflict candidates, profile idempotence,
and tool version detection. Add a failure fixture whose exit status survives
the adapter unchanged.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/adapters -run 'Debian|Arch|Root|Installer'`

Expected: FAIL because the adapters do not exist.

- [ ] **Step 3: Implement the adapters**

Keep package names, repository templates, and official installer URLs in
platform-specific code. Reuse shared tool IDs and runner interfaces rather
than duplicating planner logic.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/adapters`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/adapters
git commit -m "feat: add Debian and Arch tool adapters"
```

## Task 5: Implement the Windows adapter

**Files:**

- Create: `internal/adapters/windows.go`, `internal/adapters/windows_test.go`

**Interfaces:**

- Reuse the `Adapter` interface plus `runner.Runner` and `runner.Elevation`
  from Task 3.
- Expose `NewWindowsAdapter(runner runner.Runner, elevation runner.Elevation) Adapter`.

Use native Windows package/install sources: WinGet for packages it supports,
Docker Desktop rather than Docker Engine service management, Windows-compatible
nvm for Node, and PowerShell/official installers for tools not provided by
WinGet. Do not invoke Bash or Linux nvm paths on Windows. Keep package IDs and
installer URLs in a single map so live detection, installation, and update use
the same source.

- [ ] **Step 1: Write failing tests**

Assert WinGet command arguments for Git, GitHub CLI, Docker Desktop, and the
Windows Node manager; assert that missing tools are detected without being
shown by update; assert that elevation is requested only for system changes;
and assert that an installer failure preserves its exit code.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/adapters -run Windows`

Expected: FAIL because the Windows adapter does not exist.

- [ ] **Step 3: Implement the Windows adapter**

Use `runner.Runner` for every external command and keep PowerShell invocation
in an argument-safe helper. Treat native Windows and WSL as distinct runtime
environments. Return a clear unsupported-provider error when a tool has no
Windows installation path rather than silently installing a Linux tool.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/adapters -run Windows`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/adapters/windows.go internal/adapters/windows_test.go
git commit -m "feat: add Windows tool adapter"
```

## Task 6: Add interactive selection, version tables, and install/update execution

**Files:**

- Create: `internal/render/select.go`, `internal/render/select_test.go`, `internal/render/table.go`
- Create: `internal/install/install.go`, `internal/install/install_test.go`
- Modify: `internal/plan/plan.go`

**Interfaces:**

- Import `io`, `internal/tools`, `internal/platform`, and
  `internal/adapters` for the types below.

```go
type SelectionUI interface {
    Select(ctx context.Context, items []Item) ([]tools.ToolID, error)
}

type Item struct {
    Tool     tools.Tool
    Label    string
    Selected bool
}

type Action string
const (Install Action = "install"; Update Action = "update")

type ToolStatus struct {
    Tool tools.Tool
    Installed bool
    Selected bool
    CurrentVersion string
    CandidateVersion string
}

type Options struct {
    Yes       bool
    DryRun    bool
    Writer    io.Writer
    Selection SelectionUI
}

type ToolResult struct {
    Tool   tools.Tool
    Action Action
    Status string
    Err    error
}

type Summary struct {
    Results []ToolResult
    Failed  bool
}

func Run(ctx context.Context, action Action, statuses []ToolStatus, adapters map[platform.OS]adapters.Adapter, opts Options) Summary
```

Interactive install shows all merged profile tools selected by default and
allows deselection. Interactive update receives only installed tools, shows
current/candidate versions, and selects every item by default. `--yes` skips
selection and confirmation; `--dry-run` renders but does not call adapters.

- [ ] **Step 1: Write failing tests**

Cover install deselection, update filtering of missing tools, all-installed
defaults, `--yes`, `--dry-run`, version table formatting, duplicate tools,
latest-version no-ops, dependency ordering, and non-zero summary behavior.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render ./internal/install ./internal/plan`

Expected: FAIL because the selection and executor do not exist.

- [ ] **Step 3: Implement the selection renderer and executor**

Use a portable numbered selection prompt first; keep the `SelectionUI` boundary
open for a richer TUI later. Execute each selected tool once in dependency
order, continue independent tools after a failure, skip dependents, and return
a summary with a non-zero error when any requested action fails.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/render ./internal/install ./internal/plan`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/render internal/install internal/plan
git commit -m "feat: add interactive tool plans and execution"
```

## Task 7: Wire the Cobra `jb tools` commands

**Files:**

- Create: `internal/cli/tools.go`, `internal/cli/tools_test.go`
- Modify: `internal/cli/root.go`, `internal/cli/root_test.go`

Implement these commands and flags:

```text
jb tools install --profiles=<name>[,<name>...] [--only=<tool>[,<tool>...]] [--yes] [--dry-run]
jb tools update [--profiles=<name>[,<name>...]] [--only=<tool>[,<tool>...]] [--yes] [--dry-run]
```

`install` requires profiles or explicit tools (`--only` is the explicit-tool
path). `update` may omit profiles and then scans every supported installed
tool. Profile parsing trims whitespace, rejects empty names, and reports
unknown names before any mutation.

- [ ] **Step 1: Write failing command tests**

Test command parsing, profile union passed to the planner, update-without-
profile live scan, `--only` narrowing, `--yes`, `--dry-run`, and clear errors
for missing selection input.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli`

Expected: FAIL because the `tools` command tree is missing.

- [ ] **Step 3: Implement the Cobra command wiring**

Inject a service interface into the command layer so command tests use fixture
adapters and do not touch the host. Keep `version`, `help`, and `completion`
working from the root command.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli ./internal/plan ./internal/install`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add cmd/jb internal/cli
git commit -m "feat: add jb tools install and update commands"
```

## Task 8: Replace publication assets and documentation

**Files:**

- Create: `install.sh`, `install.ps1`
- Modify: `CNAME`, `README.md`, `.nojekyll`, `tests/publication.bash`
- Delete: `linux/dev-server/setup.sh`, `tests/dev-server-setup.bash`
- Create: `tests/cli-smoke.sh`, `tests/cli-smoke.ps1`

The install scripts detect OS and architecture, download the matching GitHub
Release asset, verify its checksum, install `jb` into a user-writable location,
and print the PATH action required by the current shell. The PowerShell script
must request elevation only when writing to a machine-wide location; the Linux
script must install without root when a user-local bin directory is available.

Update `CNAME` to `cli.johanbostrom.se` and rewrite the README around Johan
Bostrom CLI branding, profile selection and deduplication, live updates,
Debian/Ubuntu, Arch, Windows, privilege behavior, GitHub Releases, and both
bootstrap URLs. Document macOS as future support, not an initial guarantee.

- [ ] **Step 1: Write failing publication tests**

Assert the new CNAME, CLI installation URLs, command examples, profile names,
stateless update wording, Windows support, and removal of the old public URL.
Smoke tests should run the installer with a fixture release server and assert
the selected binary path and checksum failure behavior.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bash tests/cli-smoke.sh` and `bash tests/publication.bash`.

Expected: FAIL because the new assets and documentation do not exist.

- [ ] **Step 3: Implement the publication assets and documentation**

Keep release URLs configurable through a documented `JB_RELEASE_BASE_URL`
override so local smoke tests never contact GitHub. Never pipe a downloaded
binary or script directly into a shell without checksum verification.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./...`, `bash tests/cli-smoke.sh`, `bash tests/publication.bash`,
and the PowerShell smoke test on Windows.

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add CNAME README.md install.sh install.ps1 tests
git rm linux/dev-server/setup.sh tests/dev-server-setup.bash
git commit -m "feat: publish jb cli installers and documentation"
```

## Task 9: Add local build and artifact verification scripts

**Files:**

- Create: `scripts/build-local.ps1`, `scripts/build-local.sh`
- Create: `scripts/check-artifacts.ps1`, `scripts/check-artifacts.sh`
- Modify: `README.md`

Build deterministic release archives for Linux amd64/arm64 and Windows
amd64/arm64 using `-trimpath` and an injected version. Each archive must
contain the `jb` executable, a SHA-256 checksum must be emitted beside it, and
the executable must answer `jb version`. Keep the build scripts independent of
GitHub Actions so releases can be prepared locally while Actions capacity is
unavailable.

- [ ] **Step 1: Write failing artifact checks**

Make the checker fail when an expected target, archive member, checksum, or
version output is missing, and when an archive contains unexpected paths.

- [ ] **Step 2: Run the checks to verify they fail**

Run: `pwsh scripts/check-artifacts.ps1` (or `bash scripts/check-artifacts.sh`)

Expected: FAIL because the build and artifact scripts do not exist.

- [ ] **Step 3: Implement cross-platform local builds and checks**

Support a version argument with a development fallback, write outputs below a
temporary or explicitly supplied directory, and avoid mutating the repository
with generated binaries. Document the commands and target matrix in the
README.

- [ ] **Step 4: Run the checks to verify they pass**

Run both script variants where their host shell is available, then run
`jb version` from each unpacked target archive.

- [ ] **Step 5: Commit**

```text
git add scripts README.md
git commit -m "build: add local release artifact checks"
```

## Task 10: Run the complete local verification and handoff

**Files:**

- Modify: none unless verification exposes a defect

Run the full local verification suite twice to catch order-dependent or
stateful behavior:

```text
go test ./...
go vet ./...
go build ./cmd/jb
pwsh scripts/build-local.ps1
pwsh scripts/check-artifacts.ps1
bash tests/cli-smoke.sh
git diff --check
```

Run the Bash build/check and PowerShell smoke variants on their supported host
when available. Inspect the final tree for old `scripts.johanbostrom.se`,
`linux/dev-server`, and shell-installer instructions. Confirm there are no
state files, telemetry calls, or untracked generated artifacts. Leave release
publishing and GitHub Actions triggering for a later user-approved turn.

- [ ] **Step 1: Run the complete suite**

Run every applicable command above and record failures with the smallest
focused fix.

- [ ] **Step 2: Repeat the complete suite**

Run the same checks again after cleanup; verify the second run produces the
same results and no repository changes.

- [ ] **Step 3: Inspect the final diff and repository state**

Use `git diff --check`, `git status --short`, and targeted searches for removed
URLs and paths. Do not push or start release workflows in this plan.

- [ ] **Step 4: Commit any verification-only fixes**

```text
git add <focused-files>
git commit -m "test: verify jb cli locally"
```
