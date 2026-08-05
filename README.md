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

### Prepare a local release

The local build scripts create the published release matrix outside the
repository by default, in the system temporary directory. Both scripts use the
verified Go executable at `.tools/go1.26.5/go/bin/go.exe`, compile with
`-trimpath`, and inject the requested version (or `dev` when omitted):

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
