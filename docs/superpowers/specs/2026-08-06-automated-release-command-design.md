# Automated Release Command Design

## Purpose

Provide one interactive, cross-platform command that selects the next version,
validates the repository, builds and verifies all release artifacts, creates the
Git tag, and publishes a GitHub release. The command must work on Windows,
Linux, and macOS and must not require a GitHub Actions release workflow.

## User Interface

The release starts with:

```text
go run ./cmd/release
```

The command reads the greatest stable semantic-version tag and presents a menu:

```text
Current version: v1.2.3
Release commit:  a1b2c3d

Select the next version:
  1. Patch   v1.2.4
  2. Minor   v1.3.0
  3. Major   v2.0.0
  4. Custom
  5. Cancel
```

When the repository has no semantic-version tags, the current version is
treated as `v0.0.0`, producing `v0.0.1`, `v0.1.0`, and `v1.0.0` as the three
calculated choices. A custom version must use canonical `vMAJOR.MINOR.PATCH`
syntax, must be greater than the current version, and must not already exist as
a tag. Prerelease and build-metadata versions are outside the initial scope.

The command is interactive-only. Missing input, end-of-file, cancellation, or
a negative final confirmation ends the command without remote changes.

## Architecture

A Go command at `cmd/release` owns the workflow so every supported host uses
the same versioning, validation, confirmation, and publication behavior.
Focused internal release packages separate semantic-version handling, command
execution, user interaction, and orchestration so the behavior can be tested
without accessing a real repository or GitHub account.

The orchestrator reuses the existing platform-specific artifact tooling:

- Windows invokes `scripts/build-local.ps1` and
  `scripts/check-artifacts.ps1` through PowerShell.
- Linux and macOS invoke `scripts/build-local.sh` and
  `scripts/check-artifacts.sh` through Bash.

Artifacts are written to a newly created temporary directory that is outside
the repository. The directory is removed after a successful publication. It is
preserved after an artifact or publication failure so the output can be
inspected and reused during recovery.

## Preconditions

Before version selection or artifact creation, the command must:

1. Locate `git`, `go`, `gh`, and the host-specific shell (`pwsh` on Windows or
   `bash` elsewhere).
2. Confirm `gh auth status` succeeds.
3. Confirm the current branch is exactly `main`.
4. Confirm the working tree and index are clean.
5. Fetch `origin/main` and all tags without changing the working tree.
6. Confirm local `HEAD` and `origin/main` identify the same commit.
7. Confirm the Git remote named `origin` is available.

Failure of any precondition stops the command before it creates a tag, draft,
or release.

## Local Validation and Artifact Preparation

After selecting a version, the command runs these checks in order:

1. `go test ./...`
2. `go vet ./...`
3. `go build ./cmd/jb`
4. The host-specific local artifact builder with the selected version and an
   explicit temporary output directory.
5. The host-specific artifact checker with the same version and directory.

The expected release set is exactly:

- `jb_linux_amd64.tar.gz`
- `jb_linux_amd64.tar.gz.sha256`
- `jb_linux_arm64.tar.gz`
- `jb_linux_arm64.tar.gz.sha256`
- `jb_windows_amd64.zip`
- `jb_windows_amd64.zip.sha256`
- `jb_windows_arm64.zip`
- `jb_windows_arm64.zip.sha256`

The release command independently checks that these eight regular files exist
and that no unexpected files will be uploaded. The existing artifact checker
remains responsible for archive membership, checksums, executable permissions,
and native `jb version` execution where the host can run the target binary.

## Confirmation and Publication

After all local checks pass, the command displays:

- The selected version.
- The full release commit identifier.
- The eight artifact names.
- That GitHub will generate the release notes.
- That the release will be published immediately.

No remote mutation occurs until the user gives an explicit final confirmation.
After confirmation, the command:

1. Creates an annotated local tag named after the selected version, pointing at
   the validated commit.
2. Pushes that exact tag to `origin`.
3. Uses `gh release create` with `--verify-tag`, `--generate-notes`, and
   `--draft` to create a draft and upload all eight assets.
4. Queries the draft through `gh` and verifies that it exists, is still a
   draft, and contains exactly the expected asset names.
5. Uses `gh release edit --draft=false` to publish it immediately.
6. Queries the published release, confirms that it is no longer a draft, and
   prints its public URL.

The draft is an internal transactional safety step. The user is not asked to
review it on GitHub, and successful execution always ends with a published
release.

## Failure and Recovery Behavior

Local failures leave Git and GitHub unchanged. After final confirmation, each
completed external step is reported as it happens.

If tag creation, tag push, draft creation, asset upload, draft verification, or
publication fails, the command stops immediately. It does not delete local or
remote tags, drafts, releases, or assets. Its error output states which steps
completed, preserves the artifact directory, and prints a concrete recovery
command appropriate to the observed state. Examples include retrying the tag
push, inspecting the draft with `gh release view`, uploading a missing asset
with `gh release upload`, or publishing a verified draft with
`gh release edit --draft=false`.

The initial implementation does not attempt automatic resume or rollback.
Preserving observable state is safer than guessing whether a failed remote
operation completed.

## Testing

Unit tests cover:

- Stable semantic-version parsing and canonical formatting.
- Patch, minor, and major calculation, including the no-tag case.
- Rejection of malformed, non-increasing, prerelease, build-metadata, and
  duplicate custom versions.
- Menu selection, cancellation, end-of-file, and final confirmation.
- Exact artifact-set validation.
- Release-state decisions used for diagnostics and recovery guidance.

Orchestration tests use temporary repositories and fake executables for Git,
Go, GitHub CLI, PowerShell, and Bash. They verify command order, passed
arguments, platform-specific script selection, output directory handling, and
failure propagation. They must prove that no tag or GitHub mutation command is
issued before successful local validation and explicit confirmation.

Failure tests cover a missing dependency, failed GitHub authentication, wrong
branch, dirty tree, stale `main`, invalid or duplicate version, failed Go
validation, incomplete artifacts, failed tag creation or push, failed draft
creation, incorrect uploaded assets, and failed publication verification.

The existing Go tests, Bash behavioral tests, publication tests, and
PowerShell smoke test remain unchanged and continue to validate the product and
artifact scripts themselves.

## Documentation

The README publishing section will identify `go run ./cmd/release` as the
standard release path, explain its preconditions and interactive choices, list
the safety checks, and retain the lower-level local build commands as a manual
recovery and troubleshooting path.

## Out of Scope

- Automatic release publication from GitHub Actions.
- Non-interactive or CI release mode.
- Inferring version changes from commit messages.
- Prerelease or build-metadata versions.
- Automatic deletion, rollback, or resume of partial remote state.
- macOS release binaries; macOS supports running the release command but the
  published binary matrix remains Linux and Windows on amd64 and arm64.
