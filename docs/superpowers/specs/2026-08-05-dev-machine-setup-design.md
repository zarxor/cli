# Dev Machine Setup and Publishing Design

## Purpose

This repository publishes general-purpose scripts at stable URLs under
`https://scripts.johanbostrom.se`. Its first script prepares a Debian- or
Arch-based development machine and can safely be rerun to update its tools.

The first public script URL is:

`https://scripts.johanbostrom.se/linux/dev-server/setup.sh`

## Repository and Publishing Architecture

The repository uses a self-contained public script. All platform detection,
installation, updates, verification, and interactive setup behavior for the
first use case live in `linux/dev-server/setup.sh`. Focused shell functions
keep the file readable without making a remote invocation depend on additional
downloads from the repository.

GitHub Pages publishes the repository contents after automated validation
passes. A root-level `CNAME` file declares `scripts.johanbostrom.se`. The public
path mirrors the repository path so URLs remain predictable and permanent.

The documented remote invocation is:

```bash
bash <(curl -fsSL https://scripts.johanbostrom.se/linux/dev-server/setup.sh)
```

Process substitution is required instead of `curl | bash` so the script's
interactive wizard retains access to the terminal's standard input.

## Supported Systems

The script supports:

- Debian-family Linux distributions with `apt`.
- Arch-family Linux distributions with `pacman`.

It reads `/etc/os-release` to select the appropriate package operations. An
unsupported operating system or missing supported package manager produces a
clear error before the script changes the machine.

## Installed Tools

Every run converges the machine on the latest stable release available through
each tool's official repository, package source, or installer. Nightly and
prerelease releases are excluded.

The script installs or updates:

- Codex CLI.
- Git.
- GitHub CLI (`gh`).
- Docker Engine and its supported command-line plugins.
- nvm.
- The latest Node.js LTS release.
- npm, upgraded to its latest stable release after Node.js is active.
- pnpm.
- Yarn.
- Bun.

System packages use distribution-appropriate supported sources. User-level
tools are installed for the invoking non-root user, including when privileged
system operations require `sudo`.

## Idempotent Execution Flow

The script uses strict shell error handling and performs these stages in order:

1. Verify that the host is Linux, required basic commands are available, a
   supported package manager exists, and required privilege escalation is
   possible.
2. Detect the Debian or Arch distribution family.
3. Refresh package metadata and install or update system-level prerequisites,
   Git, GitHub CLI, Docker, and build tools.
4. Install or update nvm, Node.js LTS, npm, pnpm, Yarn, Bun, and Codex for the
   invoking user.
5. Verify every required command and report its installed version.
6. Offer the optional interactive setup wizard.
7. Print a final summary and any required next steps, such as opening a new
   shell or logging out and back in.

Each installer detects existing state and uses the tool's supported upgrade
path. Rerunning the script updates existing installations instead of failing,
duplicating configuration, or destructively replacing user settings.

## Interactive Setup Wizard

After successful installation, the script asks whether to start the setup
wizard. Declining it leaves all installed tools available without performing
authentication or changing personal configuration.

Within the wizard, each action is described and independently skippable:

1. Configure Git user name and email. Existing values are displayed and are
   changed only after explicit confirmation.
2. Authenticate GitHub CLI using `gh auth login`.
3. Authenticate Codex using its supported interactive login flow.
4. Add the invoking user to Docker's group for non-root access. The script
   explains the privilege implications and requires explicit confirmation.
5. Install and select the latest Node.js LTS release as nvm's default.

Authentication is never attempted during the noninteractive installation
stages. The script does not collect, store, or print credentials.

## Failures and User Feedback

Required installation failures stop execution immediately with the stage and
tool named in the error. The script must not print a successful completion
summary after a required step fails.

Progress output distinguishes completed, skipped, updated, and failed actions.
The final summary reports verified versions and configuration actions taken or
skipped. Commands that require a new login session are called out explicitly.

Existing Git identity and authentication state are preserved unless the user
confirms a change. Docker group membership changes are never implicit.

## Documentation Contract

The root `README.md` is the index and operating guide for every published
script. Each script entry documents:

- Its permanent public URL and repository path.
- Purpose and supported systems.
- Tools or configuration it changes.
- Prerequisites and privilege requirements.
- A copy-paste invocation that preserves interactive input.
- What happens on first and repeated runs.
- Wizard choices and security-relevant effects.
- Troubleshooting and verification commands.

Adding or changing a public script requires updating the README in the same
change. Automated validation enforces the presence of the first script URL and
its essential usage documentation.

## Automated Validation and Deployment

GitHub Actions validates pull requests and pushes before Pages deployment. The
checks cover:

- Bash syntax.
- ShellCheck findings at the repository's enforced severity.
- Executable permissions for public shell scripts.
- Expected public paths and the custom-domain file.
- README references for each published script.
- Mocked Debian- and Arch-family detection and package-command selection.
- Idempotent decision behavior for already-present versus missing tools.

The test suite must not modify the CI host's real package state. Distribution
and tool states are simulated through isolated fixtures or mocked commands.

After validation succeeds on `main`, the deployment workflow publishes the
repository as a GitHub Pages artifact. DNS configuration for
`scripts.johanbostrom.se` is an external prerequisite and is documented, but is
not managed by this repository.

## Scope Boundaries

The first release does not support macOS, Windows, non-Debian/non-Arch Linux
families, unattended authentication, nightly tool releases, workstation dotfile
management, or removal of installed tools. Future scripts can use additional
paths without changing the public URL or behavior of the first script.
