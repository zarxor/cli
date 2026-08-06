# Johan Bostrom CLI Design

## Product

The project becomes **Johan Bostrom CLI**, installed as the `jb` command and
documented at `https://cli.johanbostrom.se`.

The existing Linux-only setup script is replaced rather than kept as a
compatibility entry point. The first CLI feature is development-machine setup;
the command hierarchy is intentionally extensible for future features.

## Goals

- Provide one cross-platform CLI for Debian-family Linux, Arch Linux, and
  Windows.
- Install and update development tools idempotently.
- Let users select profiles and individual tools before changes are made.
- Make updates stateless: inspect installed tools and versions live on every
  invocation.
- Ship standalone binaries without requiring Go, Node.js, Python, or .NET on
  the target machine.
- Keep platform-specific package-manager behavior behind explicit adapters.
- Publish documentation and bootstrap installers at
  `cli.johanbostrom.se`.
- Leave room for future commands beyond machine setup.

## Non-goals for the first release

- macOS support. The architecture must leave room for it, but no macOS adapter
  is required initially.
- Persistent installation state, telemetry, or selection history.
- Backwards compatibility with `scripts.johanbostrom.se` or the existing
  `linux/dev-server/setup.sh` URL.
- GitHub Actions release automation while the project owner is out of Actions
  capacity. Release workflows can be added later without changing the CLI
  command contract.

## Technology and distribution

- Go for the CLI binary.
- Cobra for commands, flags, help, and shell completion.
- GitHub Releases for versioned binaries and checksums.
- GitHub Pages for documentation and static bootstrap installers.
- `https://cli.johanbostrom.se/install.sh` for Linux shells (with future
  macOS support using the same bootstrap URL).
- A matching PowerShell installer at
  `https://cli.johanbostrom.se/install.ps1` for Windows.

The initial release matrix targets Linux amd64/arm64 and Windows amd64/arm64.
The build layout should make adding macOS amd64/arm64 later a release-matrix
change rather than a command redesign.

## Command model

The initial public commands are:

```text
jb tools install --profiles=development
jb tools update
jb version
jb help
jb completion
```

Future commands such as `jb doctor`, `jb self-update`, and additional feature
families can be added under the same root command.

### Profile selection

Profiles are built into the CLI and never persisted locally. The initial
profile is `development` and contains:

- Git
- GitHub CLI
- Docker, Buildx, and Compose
- Codex
- Node.js LTS with nvm, npm, Corepack, pnpm, and Yarn
- Bun

Multiple profiles are accepted as a comma-separated list:

```bash
jb tools install --profiles=development,workstation
jb tools update --profiles=development,workstation
```

The CLI converts the selected profiles into a union of stable tool IDs before
planning any action. If two profiles contain the same tool, it appears once,
is installed once, and is updated once. Dependencies are deduplicated in the
same planning step.

### Installation interaction

Without `--yes`, `jb tools install` shows the merged profile tool list before
executing anything. All profile tools are selected by default, and the user
can deselect any item. Only the final selection is installed or updated.

Supported controls include:

```text
--profiles=<name>[,<name>...]
--only=<tool>[,<tool>...]
--yes
--dry-run
```

`--yes` skips the selection and confirmation prompts and applies all tools in
the merged profiles, or only the tools named by `--only` when that filter is
present. `--dry-run` always shows the plan and performs no changes.
Non-interactive use must identify a profile or explicit tool selection so a
bare automation invocation cannot install an accidental default set.

### Live update interaction

`jb tools update` discovers supported tools on the current machine every time;
it does not read a state file. With no profile filter it considers all
supported tools that are currently installed. With `--profiles`, it considers
only installed tools that belong to the selected profile union. Missing tools
are not displayed by the update command.

Before confirmation, the CLI shows each installed tool, its current version,
and the available version when the platform or official updater can resolve
one. Installed tools are pre-selected, including tools already at the latest
version; latest tools result in a clear no-op result. Users can deselect any
tool unless `--yes` is supplied.

```bash
jb tools update
jb tools update --profiles=development,workstation
jb tools update --only=node,bun
jb tools update --yes
jb tools update --dry-run
```

`jb tools update` updates development tools. Updating the `jb` binary itself is
reserved for a future `jb self-update` command so the two concerns remain
unambiguous.

## Architecture

The implementation is split into small boundaries:

1. **Command layer** — Cobra command definitions, flags, help, and exit codes.
2. **Profile catalog** — static profile definitions and stable tool IDs.
3. **Planner** — profile union, dependency expansion, deduplication, selection,
   and a dry-run plan.
4. **Detector** — live platform, installation, current-version, and candidate-
   version detection.
5. **Platform adapters** — Debian/Ubuntu, Arch, and Windows package and
   installer operations.
6. **Executor** — ordered, observable actions with confirmation, privilege
   handling, cleanup, and per-tool results.
7. **Renderer** — consistent interactive tables, progress, summaries, and
   machine-readable errors where practical.

Tool definitions expose `detect`, `current version`, `candidate version`,
`install`, `update`, and `verify` operations. The planner operates on these
interfaces instead of embedding platform branches in Cobra commands.

### Platform behavior

- Debian/Ubuntu use apt and supported GitHub CLI and Docker repositories.
- Arch uses pacman transactions and the official package sources.
- Windows uses native PowerShell and supported Windows package/install sources;
  Docker is treated as Docker Desktop and Node uses the Windows-compatible nvm
  approach rather than the Linux nvm script.
- Linux supports both root execution and normal users with `sudo`.
- Windows requests elevation only for actions that require it.
- Codex is handled through the platform-specific supported installation path;
  native Windows and WSL are treated as distinct execution environments.

## Safety and idempotence

- Every action is planned before mutation when interactive mode is used.
- Existing installations are detected and updated rather than duplicated.
- Repository definitions, PATH/profile entries, and shell configuration are
  written idempotently.
- Commands are executed through structured argument arrays, not interpolated
  shell strings.
- Downloaded installers are temporary, cleaned up on success or failure, and
  verified where the upstream release provides checksums.
- Privilege escalation is explicit and limited to the system operation that
  needs it.
- No state database, telemetry, or hidden network service is introduced.

## Errors and exit behavior

The CLI prints a per-tool result and a final summary. Independent tools may
continue after a failure, but dependent actions are skipped when their
prerequisite fails. The command exits non-zero if any requested tool action or
verification fails. `--dry-run` exits zero after rendering a valid plan.

## Verification

The first implementation will include:

- Go unit tests for profile union, dependency expansion, selection defaults,
  version parsing, and live update filtering.
- Adapter tests using command fixtures rather than changing the host machine.
- Integration tests for Debian, Arch, and Windows command planning.
- Cross-platform local builds and smoke tests before release.
- A later GitHub Actions matrix for validation and release packaging when the
  owner is ready to spend Actions capacity.
