# Johan Bostrom CLI

Johan Bostrom CLI (`jb`) installs and updates a curated development toolchain
on Debian/Ubuntu, Arch Linux, and Windows. It is distributed as a standalone
binary through GitHub Releases; Go is not required on the target machine.

macOS is planned for the future, but it is not supported by the initial
release.

## Install `jb`

The bootstrap installers select the release for the current operating system
and CPU architecture, download its SHA-256 checksum, verify the archive, and
only then install `jb`. The one-line commands below execute a downloaded
bootstrap script directly; if you prefer to review the script first, use the
download-and-review form instead.

### Debian/Ubuntu and Arch Linux

For a one-line quick install:

```bash
curl -fsSL https://cli.johanbostrom.se/install.sh | bash
```

To download and review the Linux bootstrap first, use
[`https://cli.johanbostrom.se/install.sh`](https://cli.johanbostrom.se/install.sh),
then review it and run it:

```bash
curl -fsSLO https://cli.johanbostrom.se/install.sh
less install.sh
bash install.sh
```

The default destination is `~/.local/bin/jb`. The installer creates that
user-local directory and does not need root. If the directory is not already on
`PATH`, it prints the command needed for the current shell. An explicitly
machine-wide `JB_INSTALL_DIR`, such as `/usr/local/bin`, uses `sudo` only when
the destination is not writable.

### Windows

For a one-line quick install:

```powershell
Invoke-RestMethod https://cli.johanbostrom.se/install.ps1 | Invoke-Expression
```

To download and review the PowerShell bootstrap first, use
[`https://cli.johanbostrom.se/install.ps1`](https://cli.johanbostrom.se/install.ps1),
then review it and run it:

```powershell
Invoke-WebRequest https://cli.johanbostrom.se/install.ps1 -OutFile install.ps1
Get-Content .\install.ps1
.\install.ps1
```

The default destination is inside the current user's local application data and
does not request elevation. Use `.\install.ps1 -Machine` for a machine-wide
installation; only that machine-wide write requests elevation. The installer
prints both the current-shell and persistent user `PATH` actions when needed.

Both installers default to the latest assets at GitHub Releases. Maintainers
and offline tests can set `JB_RELEASE_BASE_URL` to an alternate release root;
the selected archive and its adjacent `.sha256` file must use the same asset
names as a GitHub release.

The hosted documentation page adds a Copy button to each command block while
keeping every command available as plain text.

## Install development tools

Install from the complete supported tool catalog with:

```text
jb tools install
```

Every not-yet-installed eligible tool is preselected in the interactive list.
Already installed tools remain visible, grayed out with their detected version,
but cannot be selected by `jb tools install`:

```text
Available to install
❯ [✓] Git
  [✗] Docker

Already installed
  [-] GitHub CLI (already installed: 2.74.2)
```

Use ↑/↓ to move, Space to toggle `[✓]` selected and `[✗]` deselected tools,
and Enter to accept the plan. Escape or Ctrl+C cancels without making changes.
When input or output is redirected, `jb` uses a plain numbered selection
instead. Colors are enabled only in a terminal and can be disabled by setting
the `NO_COLOR` environment variable; the symbols keep every state readable
without color.

Use `jb tools update` when you want to update an already installed tool.

For automation, skip the selection and confirmation prompts with:

```text
jb tools install --yes
```

Add `--dry-run` to either form to render the plan without changing the machine.

The built-in `development` profile contains the supported development
toolchain, including the Claude Code, Codex, and T3 Code CLIs, and can still be
selected explicitly:

```text
jb tools install --profiles=development
jb tools install --profiles=desktop
jb tools install --profiles=server
```

Profiles are built into `jb` and are never written to disk. Multiple profiles
are supplied as a comma-separated list:

```text
jb tools install --profiles=<name>[,<name>...]
```

When neither `--profiles` nor `--only` is supplied, `jb` detects the host shape
and applies one automatic profile before planning. A regular desktop applies
`desktop`, which contains the local development toolchain, Claude Code, Codex,
T3 Code, and Bun. A headless host applies `server`, which contains Git, GitHub
CLI, Docker, Node.js, Claude Code, and Codex with their runtime dependencies.
The CLI prints the applied profile and the detection reason before checking
installed tools:

```text
• Applied profiles: desktop (auto-detected desktop: active graphical session detected)
```

The explicit `--profiles` flag overrides automatic selection. `--only` skips
profiles and directly selects the named tools. Tools and dependencies shared by
multiple profiles are deduplicated by stable tool ID before the plan is shown
or executed. Built-in profile names are `development`, `desktop`, and `server`.

Claude Code and Codex are installed as the user-level `claude` and `codex` CLI
commands on both automatic profiles. T3 Code is installed as the user-level
`t3` CLI on desktop and development profiles; use `--only=t3-code` to add it
to another explicit plan.

Use `--only` to narrow a profile to the tools you want to manage. Dependencies
needed by a narrowed tool are still added automatically:

```text
jb tools install --profiles=development --only=bun
```

This plans Bun together with its required Node.js, npm, nvm, and Git
dependencies. To start from an explicit tool list, omit `--profiles` (required
dependencies are still added):

```text
jb tools install --only=git,bun
```

## Install Agent Skills

Agent Skills are portable capability bundles centered on a `SKILL.md` file.
`jb` exposes only the skills explicitly listed in its available-skills catalog;
it does not treat arbitrary internet sources as install choices:

```text
jb skills install
```

The install flow matches `jb tools install`: before the skill list appears, `jb`
asks for an installation scope and the AI harnesses that should receive the
skills. Global scope and both Codex and Claude harnesses are the defaults;
not-yet-installed destinations are selected by default. The catalog currently
includes the stable skills from [Matt Pocock's skills
repository](https://github.com/mattpocock/skills), the `caveman` skill from
[Julius Brussee's Caveman](https://github.com/JuliusBrussee/caveman), and the
`impeccable` skill from [Impeccable](https://github.com/pbakaus/impeccable).
Use `--only` to narrow the catalog, `--yes` to select every eligible skill
without prompting, or `--dry-run` to preview the plan:

The interactive catalog is grouped by creator, so the Matt Pocock, Julius
Brussee, and Impeccable collections appear as separate sections. Installation
checks exactly the selected scope × harness combination; it cannot prove that an unknown AI
harness elsewhere on the machine has or does not have the same skill. Existing
unmanaged directories at the selected destination are protected from overwrite.

```text
jb skills install --only=<skill-id>[,<skill-id>...]
jb skills install --yes
jb skills install --dry-run
```

Global scope is the default. In an interactive run, the scope prompt appears
before the skill list, and pressing Enter keeps Global. Use project scope when
the skills should be available only in the current repository. The same setup
step lets you choose Codex, Claude, or both harnesses:

```text
jb skills install --scope=global
jb skills install --scope=project
jb skills install --harnesses=codex
jb skills install --scope=project --harnesses=codex,claude
```

Global Codex skills are installed in the user Codex skills directory and global
Claude skills in the user `.claude/skills` directory. Project Codex skills use
`.agents/skills`; project Claude skills use `.claude/skills`. The command line
never accepts an arbitrary source argument. Catalog entries own their source
and every catalog source must contain a valid
`SKILL.md` with lowercase, hyphenated `name` and a `description`.
For Impeccable, the catalog entry installs its Agent Skill bundle; upstream
provider-specific hooks and CLI/plugin assets are not installed by `jb`.

Managed skills record their catalog source and content digest in a lock
manifest. `jb skills update` checks both global and project installations for
both supported harnesses by default, shows progress without printing names
during discovery, and presents only skills whose source content changed. Use
`--harnesses=codex` or `--harnesses=claude` to limit the update scan. Selecting
a skill updates every managed scope and harness where it is installed:

```text
jb skills update
jb skills update --yes
jb skills update --dry-run
jb skills list
jb skills info <name>
jb skills verify
jb skills doctor
jb skills remove <name> --yes
```

Installation never runs scripts bundled with a skill, and existing unmanaged
skill directories are not overwritten.

## Update `jb` itself

```text
jb update
```

`jb update` downloads the latest release for the current operating system and
architecture, verifies its SHA-256 checksum, and replaces the installed CLI.
Use `jb update --dry-run` to download and validate the release without changing
the installed binary. On Windows, the final replacement completes immediately
after the command exits so the running executable can be unlocked safely.

## Run the T3 Code backend as a service

On Linux hosts with `systemd`, install the T3 Code backend as a per-user
background service with:

```text
jb service install
```

This invokes T3 Code's supported service installer. It enables the user
service, enables user lingering so it starts at boot, and starts the backend
immediately. Node.js and `npx` must already be available; install the runtime
first when needed:

```text
jb tools install --only=node
```

Use the remaining lifecycle commands to inspect or manage the service:

```text
jb service status
jb service update
jb service uninstall
```

`--base-dir=<path>` selects the T3 Code data directory, and `--dry-run`
prints the exact command without changing the host. The service command is
Linux-only because T3 Code's background service currently requires Linux with
`systemd`.

## Update installed tools

```text
jb tools update
```

For every invocation, `jb tools update` discovers installed tools and versions live,
checks current candidate versions, and shows only installed tools with an
available update. Updateable tools are selected by default before confirmation.
Updates are conservative: the active executable must match the adapter's
package provider (WinGet, apt, pacman, or the configured NVM/npm path); ambiguous
installations are left untouched.
No local state database, telemetry, or selection history is used by `jb` itself.
The T3 Code service is opt-in and is managed by T3 Code when explicitly installed.

Use `jb tools update --yes` for non-interactive updates or `jb tools update
--dry-run` to inspect the plan without changing anything.

Updates without `--profiles` or `--only` apply the same automatic desktop/server
profile detection as installation. Supplying a profile limits discovery to
that profile, while `--only` directly limits the live scan to the listed tools:

```text
jb tools update --profiles=development
jb tools update --profiles=development --only=bun
```

When no profile is supplied, `--only` directly limits the live scan to the
listed tools:

```text
jb tools update --only=docker
```

## Other commands

```text
jb version
jb completion bash
jb completion powershell
```

## Platforms and privileges

- Debian/Ubuntu uses supported apt repositories and `sudo` only for system
  package operations when the current user is not root.
- Arch Linux uses supported full `pacman` transactions and the same limited
  privilege boundary.
- Windows uses per-user package operations where possible and requests
  elevation only for actions that actually require machine-wide access.
- Root Linux runs privileged actions directly. User-owned tools and profile
  edits converge in place without duplicate shell blocks.

`jb` plans from live machine state on each run. Existing tools are updated in
place, and repeated runs remain idempotent.

## Publishing

Documentation and bootstrap installers are published at
[`cli.johanbostrom.se`](https://cli.johanbostrom.se). Versioned Linux amd64,
Linux arm64, Windows amd64, and Windows arm64 archives and their SHA-256 files
are published through GitHub Releases.

The `site/` directory contains the user-facing Pages source. The Pages workflow
copies its contents into the root of the uploaded artifact (`index.html`,
`styles.css`, installers, and `CNAME`); internal planning documents are
excluded from deployment. In repository settings, Pages must use **GitHub
Actions** as its source. Branch publishing only supports the repository root
or `/docs`, not `/site`.

The repository's `CNAME` declares the same hostname. Releases are prepared and
published locally without depending on GitHub Actions.

### Publish a release

Run the interactive release command from a clean `main` branch that is
synchronized with `origin/main`:

```text
go run ./cmd/release
```

The command requires Git, Go, the authenticated GitHub CLI (`gh`), and
PowerShell on Windows or Bash on Linux and macOS. It fetches the current tags,
shows patch, minor, major, custom-version, and cancel choices, then runs the Go
tests, vetting, build, artifact generation, and artifact verification.

After the checks pass, the command shows the selected version, exact commit,
and all eight assets. Nothing is changed remotely until the final confirmation.
Once confirmed, it creates and pushes the annotated version tag, creates a
temporary draft with GitHub-generated release notes, uploads and verifies every
asset, and immediately publishes the release. Successful runs print the public
release URL.

If a remote operation fails, the command preserves the generated artifacts and
any tag or draft that already exists. It prints the artifact directory and a
recovery command; it never deletes or overwrites remote release state
automatically.

On macOS, install GNU tar before releasing so deterministic archives can be
created:

```text
brew install gnu-tar
```

### Prepare artifacts manually

The local build scripts create the published release matrix outside the
repository by default, in the system temporary directory. The PowerShell script
uses the verified Go executable at `.tools/go1.26.5/go/bin/go.exe`; the Bash
script discovers the native `go` executable (or accepts `--go`/`JB_GO_EXE`).
Both compile with `-trimpath` and inject the requested version (or `dev` when
omitted):

```powershell
pwsh -NoProfile -File scripts/build-local.ps1 -Version v1.2.3
pwsh -NoProfile -File scripts/check-artifacts.ps1 -Version v1.2.3
```

```bash
bash scripts/build-local.sh --version v1.2.3
bash scripts/check-artifacts.sh --version v1.2.3
```

Use `-OutputDir`/`--output-dir` to supply another output directory, and pass
that same directory to `-ArtifactDir`/`--artifact-dir` for the checker. The
release assets are:

- `jb_linux_amd64.tar.gz` and `jb_linux_arm64.tar.gz`, each containing only `jb`
- `jb_windows_amd64.zip` and `jb_windows_arm64.zip`, each containing only `jb.exe`
- a matching `<asset>.sha256` file beside every archive

The checker validates every archive's expected member and SHA-256 checksum. It
also unpacks and runs `jb version` for the archive matching the current host's
operating system and CPU architecture; run it on each native platform to cover
both operating systems and architectures.

## Development

Use the repository's verified Go toolchain and run:

```bash
go test ./...
bash tests/cli-smoke.sh
bash tests/publication.bash
```

On Windows, also run:

```powershell
pwsh -NoProfile -File tests/cli-smoke.ps1
```
