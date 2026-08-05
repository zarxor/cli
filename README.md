# Johan Bostrom CLI

Johan Bostrom CLI (`jb`) installs and updates a curated development toolchain
on Debian/Ubuntu, Arch Linux, and Windows. It is distributed as a standalone
binary through GitHub Releases; Go is not required on the target machine.

macOS is planned for the future, but it is not supported by the initial
release.

## Install `jb`

The bootstrap installers select the release for the current operating system
and CPU architecture, download its SHA-256 checksum, verify the archive, and
only then install `jb`. Review a bootstrap script before running it; do not pipe
a downloaded script directly into a shell.

### Debian/Ubuntu and Arch Linux

Download the Linux bootstrap from
[`https://cli.johanbostrom.se/install.sh`](https://cli.johanbostrom.se/install.sh),
review it, and run it:

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

Download the PowerShell bootstrap from
[`https://cli.johanbostrom.se/install.ps1`](https://cli.johanbostrom.se/install.ps1),
review it, and run it:

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

## Install development tools

The built-in `development` profile contains the supported development
toolchain. Preview the plan, optionally deselect tools, and confirm the install:

```text
jb tools install --profiles=development
```

Automation can accept the full plan with `--yes` or render it without changing
the machine with `--dry-run`.

Profiles are built into `jb` and are never written to disk. Multiple profiles
are supplied as a comma-separated list:

```text
jb tools install --profiles=<name>[,<name>...]
```

The initial profile name is `development`; the syntax is ready for more built-in
profiles later. Tools and dependencies shared by multiple profiles are
deduplicated by stable tool ID before the plan is shown or executed.

## Update installed tools

```text
jb tools update
```

For every invocation, `jb tools update` discovers installed tools and versions live,
checks current candidate versions, and shows only tools that are already
installed. Installed tools are selected by default before confirmation.
No local state database is used; neither are telemetry, selection history, or a
hidden service.

Use `jb tools update --yes` for non-interactive updates or `jb tools update
--dry-run` to inspect the plan without changing anything.

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

The repository's `CNAME` declares the same hostname. Release automation remains
deferred; builds and verification run locally without triggering GitHub Actions.

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
