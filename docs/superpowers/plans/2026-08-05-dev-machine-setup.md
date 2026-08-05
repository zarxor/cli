# Dev Machine Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish an idempotent Debian/Arch development-machine bootstrap script at `https://scripts.johanbostrom.se/linux/dev-server/setup.sh`, with an optional configuration wizard, automated validation, documentation, and GitHub Pages deployment.

**Architecture:** A single sourceable Bash entry point owns platform detection, package installation, user-tool updates, verification, and the wizard. Tests source the script with `DEV_SETUP_TEST_MODE=1` and replace external commands through a temporary `PATH`, allowing distribution and idempotency behavior to be exercised without changing the test host. GitHub Actions validates the repository before uploading its static contents to GitHub Pages.

**Tech Stack:** Bash 4.4+, standard GNU/Linux utilities, apt/pacman, ShellCheck, GitHub Actions, GitHub Pages.

## Global Constraints

- Public URL: `https://scripts.johanbostrom.se/linux/dev-server/setup.sh`.
- Supported hosts: Debian-family Linux with `apt`, and Arch-family Linux with `pacman`.
- Every run installs missing tools and updates existing tools through stable official sources; nightly and prerelease builds are excluded.
- Installed tools: Codex CLI, Git, GitHub CLI, Docker Engine with Buildx and Compose, nvm, latest Node.js LTS, latest npm, pnpm, Yarn, and Bun.
- The script must be run by a non-root user with working `sudo`; user-level tools belong to that invoking user.
- The optional wizard must independently offer Git identity, GitHub authentication, Codex authentication, Docker group membership, and the nvm default Node.js LTS alias.
- Existing identity and authentication state is preserved unless the user explicitly chooses a change.
- The root README is updated in the same change as every public script.
- Remote usage must use `bash <(curl -fsSL URL)`, not `curl | bash`, so wizard input remains attached to the terminal.
- Tests must not change the CI runner's actual packages, users, groups, services, authentication, or home directory.

---

## File Map

- `linux/dev-server/setup.sh`: the complete public installer/updater and wizard.
- `tests/test_helper.bash`: dependency-free assertion, fixture, command-stub, and test-runner helpers.
- `tests/dev-server-setup.bash`: unit and orchestration tests that source the public script.
- `tests/publication.bash`: repository-contract checks for public paths, modes, CNAME, and README coverage.
- `tests/run.bash`: one local/CI entry point for all test files.
- `.github/workflows/validate.yml`: syntax, ShellCheck, behavioral, and publication checks.
- `.github/workflows/pages.yml`: validation-gated Pages artifact upload and deployment from `main`.
- `CNAME`: the GitHub Pages custom domain.
- `.nojekyll`: disables Jekyll transformation so shell files are served byte-for-byte.
- `README.md`: script index, usage, behavior, security notes, verification, and DNS/Pages setup.

---

### Task 1: Test Harness and Runtime Guardrails

**Files:**
- Create: `tests/test_helper.bash`
- Create: `tests/dev-server-setup.bash`
- Create: `tests/run.bash`
- Create: `linux/dev-server/setup.sh`

**Interfaces:**
- Consumes: `/etc/os-release`-format files and standard Bash environment variables.
- Produces: `detect_platform`, `require_non_root`, `require_supported_host`, `run`, `log`, `die`, and a source-safe `main` guard.
- Test controls: `OS_RELEASE_FILE`, `DEV_SETUP_TEST_MODE`, `DEV_SETUP_COMMAND_LOG`, `DEV_SETUP_USER`, and `DEV_SETUP_HOME`.

- [ ] **Step 1: Write the failing platform and privilege tests**

Create a small test harness whose core functions are exactly:

```bash
TEST_FAILURES=0

fail() { printf 'not ok - %s\n' "$1" >&2; TEST_FAILURES=$((TEST_FAILURES + 1)); }
pass() { printf 'ok - %s\n' "$1"; }
assert_eq() {
  local expected=$1 actual=$2 message=$3
  [[ "$actual" == "$expected" ]] && pass "$message" || fail "$message (expected '$expected', got '$actual')"
}
assert_contains() {
  local haystack=$1 needle=$2 message=$3
  [[ "$haystack" == *"$needle"* ]] && pass "$message" || fail "$message (missing '$needle')"
}
```

In `tests/dev-server-setup.bash`, create temporary Debian and Arch `os-release` fixtures, set `DEV_SETUP_TEST_MODE=1`, source `linux/dev-server/setup.sh`, and assert:

```bash
OS_RELEASE_FILE="$TEST_TMP/debian-os-release"
detect_platform
assert_eq debian "$PLATFORM" "detects Debian family"

OS_RELEASE_FILE="$TEST_TMP/arch-os-release"
detect_platform
assert_eq arch "$PLATFORM" "detects Arch family"

OS_RELEASE_FILE="$TEST_TMP/fedora-os-release"
if (detect_platform >/dev/null 2>&1); then
  fail "rejects unsupported distributions"
else
  pass "rejects unsupported distributions"
fi
```

Also exercise a subprocess with `DEV_SETUP_EFFECTIVE_UID=0` and assert that it exits nonzero with `Run this script as a non-root user`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because the public script and detection functions do not exist.

- [ ] **Step 3: Implement the sourceable shell foundation**

Start `setup.sh` with:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
DEV_SETUP_USER=${DEV_SETUP_USER:-${SUDO_USER:-$USER}}
DEV_SETUP_HOME=${DEV_SETUP_HOME:-$HOME}
DEV_SETUP_EFFECTIVE_UID=${DEV_SETUP_EFFECTIVE_UID:-$EUID}
PLATFORM=
DISTRO_ID=
DISTRO_ID_LIKE=
DISTRO_VERSION_CODENAME=
DISTRO_UBUNTU_CODENAME=
DISTRO_DEBIAN_CODENAME=

log() { printf '[dev-setup] %s\n' "$*"; }
die() { printf '[dev-setup] ERROR: %s\n' "$*" >&2; return 1; }
run() {
  if [[ -n "${DEV_SETUP_COMMAND_LOG:-}" ]]; then
    printf '%q ' "$@" >>"$DEV_SETUP_COMMAND_LOG"
    printf '\n' >>"$DEV_SETUP_COMMAND_LOG"
  fi
  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] || "$@"
}

detect_platform() {
  [[ -r "$OS_RELEASE_FILE" ]] || die "Cannot read $OS_RELEASE_FILE"
  local ID= ID_LIKE= VERSION_CODENAME= UBUNTU_CODENAME= DEBIAN_CODENAME=
  # shellcheck disable=SC1090
  . "$OS_RELEASE_FILE"
  DISTRO_ID=${ID:-}
  DISTRO_ID_LIKE=${ID_LIKE:-}
  DISTRO_VERSION_CODENAME=${VERSION_CODENAME:-}
  DISTRO_UBUNTU_CODENAME=${UBUNTU_CODENAME:-}
  DISTRO_DEBIAN_CODENAME=${DEBIAN_CODENAME:-}
  case " ${ID:-} ${ID_LIKE:-} " in
    *" arch "*) PLATFORM=arch ;;
    *" debian "*|*" ubuntu "*) PLATFORM=debian ;;
    *) die "Unsupported Linux distribution: ${ID:-unknown}" ;;
  esac
}

require_non_root() {
  [[ "$DEV_SETUP_EFFECTIVE_UID" -ne 0 ]] || die "Run this script as a non-root user; the script will request sudo when needed."
}

require_supported_host() {
  [[ "$(uname -s)" == Linux ]] || die "Only Linux is supported."
  require_non_root
  command -v sudo >/dev/null || die "sudo is required."
  detect_platform
}

main() {
  require_supported_host
}

if [[ "${BASH_SOURCE[0]}" == "$0" && "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
  main "$@"
fi
```

Keep all mutable paths and external execution behind the declared environment overrides or `run` so later tests can remain isolated.

- [ ] **Step 4: Run the guardrail tests**

Run: `bash tests/run.bash`

Expected: PASS for Debian detection, Arch detection, unsupported-host rejection, and root rejection.

- [ ] **Step 5: Commit the runtime foundation**

```bash
git add linux/dev-server/setup.sh tests/test_helper.bash tests/dev-server-setup.bash tests/run.bash
git commit -m "feat: add dev setup runtime guardrails"
```

---

### Task 2: Distribution Package Installation and Updates

**Files:**
- Modify: `linux/dev-server/setup.sh`
- Modify: `tests/dev-server-setup.bash`

**Interfaces:**
- Consumes: `PLATFORM`, `run`, `DISTRO_ID`, `DISTRO_ID_LIKE`, `DISTRO_VERSION_CODENAME`, `DISTRO_UBUNTU_CODENAME`, and `DISTRO_DEBIAN_CODENAME` from `detect_platform`.
- Produces: `install_system_packages`, `install_debian_packages`, `install_arch_packages`, `configure_github_cli_apt`, and `configure_docker_apt`.

- [ ] **Step 1: Write failing command-selection tests**

Use `DEV_SETUP_COMMAND_LOG` and assert that Debian selection records commands containing:

```text
apt-get update
apt-get install -y ca-certificates curl git gnupg build-essential
https://cli.github.com/packages/githubcli-archive-keyring.gpg
https://download.docker.com/linux/debian/gpg
docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

Assert that Arch selection records:

```text
pacman -Syu --noconfirm --needed base-devel ca-certificates curl git github-cli docker docker-buildx docker-compose
systemctl enable --now docker.service
```

Add a rerun test that invokes the same platform function twice and verifies repository files are written with replacement semantics (`tee`/`install`), never appended with `tee -a` or `>>`.

- [ ] **Step 2: Run the package tests to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because distribution package functions are undefined.

- [ ] **Step 3: Implement Debian repository and package convergence**

Implement `configure_github_cli_apt` from GitHub CLI's supported Debian repository instructions: create `/etc/apt/keyrings`, replace the keyring from `https://cli.github.com/packages/githubcli-archive-keyring.gpg`, and replace `/etc/apt/sources.list.d/github-cli.list` with the stable repository and `dpkg --print-architecture`.

Implement `configure_docker_apt` with an explicit Debian-family map. Ubuntu
derivatives use `UBUNTU_CODENAME` when provided; other Debian derivatives use
their declared Debian-compatible codename:

```bash
case " $DISTRO_ID $DISTRO_ID_LIKE " in
  *" ubuntu "*)
    DOCKER_APT_DISTRO=ubuntu
    DOCKER_APT_CODENAME=${DISTRO_UBUNTU_CODENAME:-$DISTRO_VERSION_CODENAME}
    ;;
  *" debian "*)
    DOCKER_APT_DISTRO=debian
    DOCKER_APT_CODENAME=${DISTRO_DEBIAN_CODENAME:-$DISTRO_VERSION_CODENAME}
    ;;
  *)
    die "Docker's official apt repository is not mapped for distribution '$DISTRO_ID'."
    ;;
esac
[[ -n "$DOCKER_APT_CODENAME" ]] || die "No compatible Docker apt codename is declared by '$DISTRO_ID'."
```

Replace `/etc/apt/sources.list.d/docker.sources` with a deb822 source using `https://download.docker.com/linux/$DOCKER_APT_DISTRO`, the resolved codename, stable component, current dpkg architecture, and `/etc/apt/keyrings/docker.asc`. If the declared compatibility codename does not exist in Docker's official repository, apt must fail clearly rather than falling back to an unrelated release.

After repository configuration, run:

```bash
sudo apt-get update
sudo apt-get install -y gh docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker.service
```

Installing named packages again is the update path; apt leaves already-current versions unchanged.

- [ ] **Step 4: Implement Arch package convergence**

Use one full-system transaction, as required for supported Arch upgrades:

```bash
sudo pacman -Syu --noconfirm --needed \
  base-devel ca-certificates curl git github-cli \
  docker docker-buildx docker-compose
sudo systemctl enable --now docker.service
```

Dispatch through `install_system_packages` based only on the detected `PLATFORM`.

- [ ] **Step 5: Run distribution tests and syntax validation**

Run: `bash -n linux/dev-server/setup.sh && bash tests/run.bash`

Expected: PASS, with command logs proving the correct apt or pacman path and replacement-only repository configuration.

- [ ] **Step 6: Commit system installers**

```bash
git add linux/dev-server/setup.sh tests/dev-server-setup.bash
git commit -m "feat: install Debian and Arch system tools"
```

---

### Task 3: User-Level Tool Installation and Updates

**Files:**
- Modify: `linux/dev-server/setup.sh`
- Modify: `tests/dev-server-setup.bash`

**Interfaces:**
- Consumes: `DEV_SETUP_HOME`, `DEV_SETUP_USER`, `run`, `curl`, and the current user's shell profile.
- Produces: `install_or_update_codex`, `install_or_update_nvm`, `load_nvm`, `install_or_update_node_tools`, `install_or_update_bun`, and `install_user_tools`.

- [ ] **Step 1: Write failing missing-versus-existing tool tests**

Stub `curl`, `git`, `codex`, `bun`, `npm`, `corepack`, and nvm in a temporary `PATH`. Assert:

- Codex always fetches `https://chatgpt.com/codex/install.sh`, because the official installer is also the updater.
- Missing Bun fetches `https://bun.sh/install`; existing Bun runs `bun upgrade --stable`.
- nvm resolves the latest stable `v*` tag from `git ls-remote --tags --refs https://github.com/nvm-sh/nvm.git`, then runs that tag's `install.sh`.
- Node runs `nvm install --lts --latest-npm` and `nvm use --lts` on every run.
- npm runs `npm install --global npm@latest`.
- Corepack runs `npm install --global corepack@latest`, `corepack enable`, `corepack prepare pnpm@latest --activate`, and `corepack prepare yarn@stable --activate`.

- [ ] **Step 2: Run user-tool tests to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because user-tool functions are undefined.

- [ ] **Step 3: Implement Codex and nvm convergence**

Download each remote installer to a `mktemp` file, install a trap that removes temporary files on return/exit, and execute only after `curl -fsSL` succeeds. Use:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh -o "$installer"
sh "$installer"
```

For nvm, resolve a stable tag without hard-coding a version:

```bash
nvm_version=$(git ls-remote --tags --refs https://github.com/nvm-sh/nvm.git 'v*' \
  | awk -F/ '{print $3}' | sort -V | tail -n1)
[[ "$nvm_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Could not resolve a stable nvm release."
curl -fsSL "https://raw.githubusercontent.com/nvm-sh/nvm/$nvm_version/install.sh" -o "$installer"
PROFILE=/dev/null NVM_DIR="$DEV_SETUP_HOME/.nvm" bash "$installer"
```

Add one marked, idempotently replaceable nvm initialization block to the user's `.bashrc`; do not duplicate it on reruns. `load_nvm` must source `$DEV_SETUP_HOME/.nvm/nvm.sh` in the current process.

- [ ] **Step 4: Implement Node ecosystem and Bun convergence**

After `load_nvm`, execute the exact stable update sequence:

```bash
nvm install --lts --latest-npm
nvm use --lts
npm install --global npm@latest
npm install --global corepack@latest
corepack enable
corepack prepare pnpm@latest --activate
corepack prepare yarn@stable --activate
```

If `$DEV_SETUP_HOME/.bun/bin/bun` exists, run `bun upgrade --stable`; otherwise download `https://bun.sh/install` to a temporary file and run it with `BUN_INSTALL="$DEV_SETUP_HOME/.bun" bash "$installer"`. Add Bun's bin directory to the current `PATH` and idempotently add its marked initialization line to `.bashrc`.

- [ ] **Step 5: Run user-tool tests and syntax validation**

Run: `bash -n linux/dev-server/setup.sh && bash tests/run.bash`

Expected: PASS for installer URLs, stable update commands, version-tag validation, and repeat-safe profile edits.

- [ ] **Step 6: Commit user-level installers**

```bash
git add linux/dev-server/setup.sh tests/dev-server-setup.bash
git commit -m "feat: install and update developer CLI tools"
```

---

### Task 4: Optional Interactive Setup Wizard

**Files:**
- Modify: `linux/dev-server/setup.sh`
- Modify: `tests/dev-server-setup.bash`

**Interfaces:**
- Consumes: verified `git`, `gh`, `codex`, `docker`, and loaded `nvm`; scripted answers through standard input in tests.
- Produces: `confirm`, `configure_git_identity`, `configure_github_auth`, `configure_codex_auth`, `configure_docker_access`, `configure_node_default`, and `run_wizard`.

- [ ] **Step 1: Write failing wizard tests**

Feed deterministic answer streams into `run_wizard` and assert:

- Answering no to the initial wizard prompt runs no configuration command.
- Every step can be skipped independently.
- Existing Git name/email are displayed, and no `git config --global` command runs unless replacement is confirmed.
- `gh auth status` success skips login unless the user requests reauthentication; unauthenticated state offers `gh auth login`.
- `codex login status` success skips login unless requested; unauthenticated state offers `codex login`.
- Docker membership checks `id -nG "$DEV_SETUP_USER"`; confirmed setup runs `sudo usermod -aG docker "$DEV_SETUP_USER"` only when membership is absent.
- Node setup runs `nvm alias default 'lts/*'` only after confirmation.

- [ ] **Step 2: Run wizard tests to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because wizard functions are undefined.

- [ ] **Step 3: Implement confirmation and identity/authentication steps**

Use a default-no confirmation helper:

```bash
confirm() {
  local prompt=$1 answer
  printf '%s [y/N] ' "$prompt"
  IFS= read -r answer
  [[ "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}
```

For Git, read existing values with `git config --global --get user.name` and `user.email`, prompt for new nonempty values only after confirmation, echo the proposed values, and require a final confirmation before writing both keys.

For authentication, use the supported status and login interfaces:

```bash
gh auth status
gh auth login
codex login status
codex login
```

Never read tokens, API keys, or passwords inside the script.

- [ ] **Step 4: Implement Docker and Node configuration steps**

Before Docker membership changes, print: `Membership in the docker group grants root-level privileges.` Require explicit confirmation, create the group only if `getent group docker` fails, and add only `DEV_SETUP_USER`. Set a summary flag that later prints `Log out and back in before using Docker without sudo.`

For Node, show the active LTS version and require confirmation before `nvm alias default 'lts/*'`.

- [ ] **Step 5: Run all wizard tests**

Run: `bash -n linux/dev-server/setup.sh && bash tests/run.bash`

Expected: PASS for opt-out, independent skips, preservation of existing state, supported login commands, and Docker privilege disclosure.

- [ ] **Step 6: Commit the wizard**

```bash
git add linux/dev-server/setup.sh tests/dev-server-setup.bash
git commit -m "feat: add optional development setup wizard"
```

---

### Task 5: Orchestration, Verification, and Failure Reporting

**Files:**
- Modify: `linux/dev-server/setup.sh`
- Modify: `tests/dev-server-setup.bash`

**Interfaces:**
- Consumes: all installers and wizard functions from Tasks 1-4.
- Produces: `verify_tools`, `print_summary`, a complete `main`, and nonzero failure propagation.

- [ ] **Step 1: Write failing orchestration tests**

Stub every external operation and assert the ordered phase markers are:

```text
Checking host
Installing or updating system tools
Installing or updating user tools
Verifying installed tools
Optional setup wizard
Setup complete
```

Make one required installer stub return 1 and assert that the subprocess exits nonzero, names the failed phase, never enters the wizard, and never prints `Setup complete`.

Assert that `verify_tools` reports versions for `git`, `gh`, `docker`, `docker compose`, `node`, `npm`, `pnpm`, `yarn`, `bun`, and `codex`, while verifying nvm with `command -v nvm` because nvm is a shell function.

- [ ] **Step 2: Run orchestration tests to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because complete orchestration and verification are absent.

- [ ] **Step 3: Implement phased execution and error context**

Add a current-phase variable and error trap:

```bash
CURRENT_PHASE=initialization
on_error() {
  local exit_code=$?
  printf '[dev-setup] ERROR: %s failed (exit %d).\n' "$CURRENT_PHASE" "$exit_code" >&2
  exit "$exit_code"
}
trap on_error ERR
```

Update `main` to set and log each phase before calling `require_supported_host`, `install_system_packages`, `install_user_tools`, `verify_tools`, and `run_wizard`. Print success only after every required function returns zero.

- [ ] **Step 4: Implement version verification and summary**

Use each tool's non-mutating version command and fail if a required command cannot be resolved. Normalize output to one concise line per tool. Include Docker Compose through `docker compose version`, nvm through `nvm --version`, and Codex through `codex --version`.

The summary must distinguish installed/updated tools from skipped wizard actions and print pending logout/shell-reload notes exactly once.

- [ ] **Step 5: Run the complete behavioral suite twice**

Run: `bash -n linux/dev-server/setup.sh && bash tests/run.bash && bash tests/run.bash`

Expected: both runs PASS, demonstrating deterministic, repeat-safe decisions in the mocked environment.

- [ ] **Step 6: Commit orchestration and reporting**

```bash
git add linux/dev-server/setup.sh tests/dev-server-setup.bash
git commit -m "feat: verify and summarize dev machine setup"
```

---

### Task 6: Public Documentation and Repository Contract

**Files:**
- Create: `tests/publication.bash`
- Create: `CNAME`
- Create: `.nojekyll`
- Modify: `README.md`
- Modify: `tests/run.bash`
- Modify mode: `linux/dev-server/setup.sh` to executable

**Interfaces:**
- Consumes: public script paths and README content.
- Produces: a documented, executable public asset and a test-enforced documentation contract.

- [ ] **Step 1: Write failing publication checks**

In `tests/publication.bash`, assert:

```bash
[[ -x linux/dev-server/setup.sh ]]
[[ "$(<CNAME)" == scripts.johanbostrom.se ]]
grep -Fq 'https://scripts.johanbostrom.se/linux/dev-server/setup.sh' README.md
grep -Fq 'bash <(curl -fsSL https://scripts.johanbostrom.se/linux/dev-server/setup.sh)' README.md
grep -Fq 'Debian' README.md
grep -Fq 'Arch' README.md
grep -Fq 'rerun' README.md
grep -Fq 'docker group grants root-level privileges' README.md
```

Add this file to `tests/run.bash` so failure contributes to the final test exit code.

- [ ] **Step 2: Run publication checks to verify they fail**

Run: `bash tests/run.bash`

Expected: FAIL because CNAME, documentation, and executable mode are missing.

- [ ] **Step 3: Write the README as the script index and operating guide**

Document, with exact copy-paste commands:

```bash
bash <(curl -fsSL https://scripts.johanbostrom.se/linux/dev-server/setup.sh)
```

and the auditable download-first alternative:

```bash
curl -fsSLO https://scripts.johanbostrom.se/linux/dev-server/setup.sh
less setup.sh
bash setup.sh
```

Include supported Debian/Ubuntu and Arch-family scope, all installed tools, sudo/non-root prerequisites, first-run and rerun behavior, all five wizard choices, the Docker root-equivalent privilege warning, version verification commands, troubleshooting, contribution rules requiring README updates, Pages activation, domain verification, and DNS instructions for a `scripts` CNAME record pointing to `zarxor.github.io`.

- [ ] **Step 4: Add Pages metadata and executable mode**

Set `CNAME` to exactly `scripts.johanbostrom.se`, add an empty `.nojekyll`, and run:

```bash
chmod +x linux/dev-server/setup.sh tests/run.bash tests/publication.bash
```

- [ ] **Step 5: Run publication and behavior checks**

Run: `bash tests/run.bash && git diff --check`

Expected: PASS, with the custom domain, invocation, supported systems, rerun behavior, and security warning all enforced.

- [ ] **Step 6: Commit documentation and publication metadata**

```bash
git add README.md CNAME .nojekyll linux/dev-server/setup.sh tests/run.bash tests/publication.bash
git commit -m "docs: publish dev setup usage and domain"
```

---

### Task 7: CI Validation and GitHub Pages Deployment

**Files:**
- Create: `.github/workflows/validate.yml`
- Create: `.github/workflows/pages.yml`

**Interfaces:**
- Consumes: `linux/dev-server/setup.sh`, `tests/run.bash`, static repository contents, and GitHub's `github-pages` environment.
- Produces: required validation on pushes/PRs and Pages deployment on validated `main` pushes.

- [ ] **Step 1: Add the validation workflow**

Create `validate.yml` triggered on pull requests and pushes. Use `actions/checkout@v6`, install ShellCheck through apt, and run exactly:

```bash
bash -n linux/dev-server/setup.sh tests/*.bash
shellcheck --severity=warning linux/dev-server/setup.sh tests/*.bash
bash tests/run.bash
git diff --check
```

Name the job `validate` and grant only `contents: read`.

- [ ] **Step 2: Add the validation-gated Pages workflow**

Create `pages.yml` triggered on pushes to `main` and manual dispatch. Give the workflow `contents: read`, `pages: write`, and `id-token: write`. Define:

1. A `validate` job that repeats the same syntax, ShellCheck, test, and whitespace commands so deployment cannot bypass checks.
2. A `deploy` job with `needs: validate`, `environment.name: github-pages`, `environment.url: ${{ steps.deployment.outputs.page_url }}`, and concurrency group `pages` with in-progress cancellation disabled.
3. Deployment steps using `actions/checkout@v6`, `actions/configure-pages@v5`, `actions/upload-pages-artifact@v4` with `path: .`, and `actions/deploy-pages@v4` with step id `deployment`.

- [ ] **Step 3: Validate workflow structure locally**

Run:

```bash
grep -Fq 'needs: validate' .github/workflows/pages.yml
grep -Fq 'actions/configure-pages@v5' .github/workflows/pages.yml
grep -Fq 'actions/upload-pages-artifact@v4' .github/workflows/pages.yml
grep -Fq 'actions/deploy-pages@v4' .github/workflows/pages.yml
bash tests/run.bash
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 4: Commit CI and deployment**

```bash
git add .github/workflows/validate.yml .github/workflows/pages.yml
git commit -m "ci: validate and deploy scripts to Pages"
```

---

### Task 8: Final Verification

**Files:**
- Verify only; modify earlier files only if a check exposes a defect.

**Interfaces:**
- Consumes: the completed repository.
- Produces: evidence that the public script, documentation contract, and deployment configuration satisfy the approved design.

- [ ] **Step 1: Run all local static and behavioral checks**

Run:

```bash
bash -n linux/dev-server/setup.sh tests/*.bash
shellcheck --severity=warning linux/dev-server/setup.sh tests/*.bash
bash tests/run.bash
bash tests/run.bash
git diff --check
git status --short
```

Expected: syntax and ShellCheck exit 0; both test runs pass; no whitespace errors; working tree is clean.

- [ ] **Step 2: Review the public artifact contract**

Run:

```bash
git ls-files --stage linux/dev-server/setup.sh
grep -F 'scripts.johanbostrom.se/linux/dev-server/setup.sh' README.md
grep -F 'scripts.johanbostrom.se' CNAME
```

Expected: the script mode is `100755`, README shows the permanent URL and process-substitution command, and CNAME shows the custom domain.

- [ ] **Step 3: Perform disposable-container smoke tests when Docker is available**

Run the test-mode script inside clean Debian and Arch containers with mocked `sudo` and service execution; do not install packages on the host. Expected: each container selects its own platform branch and reaches the mocked verification phase twice without duplicate profile blocks.

If Docker is unavailable locally, record that this optional smoke test was not run; the mandatory mocked distribution suite and CI remain the release gate.

- [ ] **Step 4: Enable external repository settings after pushing**

In GitHub repository settings, select GitHub Actions as the Pages source, verify `scripts.johanbostrom.se` for the owning GitHub account, configure the DNS CNAME `scripts` to `zarxor.github.io`, enable HTTPS after DNS validation, and confirm the deployment environment reports:

`https://scripts.johanbostrom.se/linux/dev-server/setup.sh`

These are explicit external steps because repository files cannot safely change account-level domain verification or DNS.
