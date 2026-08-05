#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"

TEST_TMP=$(mktemp -d)
trap 'rm -rf "$TEST_TMP"' EXIT

cat >"$TEST_TMP/debian-os-release" <<'EOF'
ID=debian
ID_LIKE=debian
VERSION_CODENAME=bookworm
EOF

cat >"$TEST_TMP/arch-os-release" <<'EOF'
ID=arch
ID_LIKE=arch
EOF

cat >"$TEST_TMP/fedora-os-release" <<'EOF'
ID=fedora
ID_LIKE="rhel fedora"
EOF

export DEV_SETUP_TEST_MODE=1
export DEV_SETUP_USER=tester
export DEV_SETUP_HOME="$TEST_TMP/home"
mkdir -p "$DEV_SETUP_HOME"

# shellcheck source=linux/dev-server/setup.sh
source "$REPO_ROOT/linux/dev-server/setup.sh"

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

set +e
root_status=$(DEV_SETUP_EFFECTIVE_UID=0 DEV_SETUP_TEST_MODE=1 \
  "$BASH" -c "source '$REPO_ROOT/linux/dev-server/setup.sh'; require_privileges" 2>&1)
root_exit=$?
set -e
assert_eq 0 "$root_exit" "root execution does not require sudo"
assert_not_contains "$root_status" "sudo is required" "root execution skips the sudo requirement"

root_commands=$(DEV_SETUP_EFFECTIVE_UID=0 DEV_SETUP_TEST_MODE=1 \
  DEV_SETUP_COMMAND_LOG="$TEST_TMP/root-commands.log" \
  "$BASH" -c "source '$REPO_ROOT/linux/dev-server/setup.sh'; run_privileged sudo apt-get update; cat '$TEST_TMP/root-commands.log'")
assert_not_contains "$root_commands" "sudo apt-get update" "root system commands do not invoke sudo"
assert_contains "$root_commands" "apt-get update" "root system commands still run directly"

DEV_SETUP_COMMAND_LOG="$TEST_TMP/commands.log"
export DEV_SETUP_COMMAND_LOG DEV_SETUP_DPKG_ARCH=amd64

: >"$DEV_SETUP_COMMAND_LOG"
OS_RELEASE_FILE="$TEST_TMP/debian-os-release"
detect_platform
export DEV_SETUP_DOCKER_CONFLICTS="docker.io docker-compose-v2 containerd"
install_system_packages
unset DEV_SETUP_DOCKER_CONFLICTS
debian_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_contains "$debian_commands" "apt-get update" "Debian refreshes apt metadata"
assert_contains "$debian_commands" "apt-get install -y ca-certificates curl git gnupg unzip build-essential" "Debian installs base development packages and Bun's unzip prerequisite"
assert_contains "$debian_commands" "https://cli.github.com/packages/githubcli-archive-keyring.gpg" "Debian configures the GitHub CLI repository"
assert_contains "$debian_commands" "https://download.docker.com/linux/debian/gpg" "Debian configures Docker's repository"
assert_contains "$debian_commands" "apt-get remove -y docker.io docker-compose-v2 containerd" "Debian and Ubuntu remove only installed packages that conflict with Docker CE"
assert_contains "$debian_commands" "docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin" "Debian converges Docker packages"
conflict_candidates=$(docker_conflict_candidates)
assert_contains "$conflict_candidates" "docker-compose-v2" "Ubuntu's Compose v2 package is included in Docker CE conflict detection"

: >"$DEV_SETUP_COMMAND_LOG"
OS_RELEASE_FILE="$TEST_TMP/arch-os-release"
detect_platform
install_system_packages
arch_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_contains "$arch_commands" "pacman -Syu --noconfirm --needed base-devel ca-certificates curl git unzip github-cli docker docker-buildx docker-compose" "Arch performs a supported transaction with Bun's unzip prerequisite"
assert_contains "$arch_commands" "systemctl enable --now docker.service" "Arch enables Docker"

: >"$DEV_SETUP_COMMAND_LOG"
OS_RELEASE_FILE="$TEST_TMP/debian-os-release"
detect_platform
install_system_packages
install_system_packages
rerun_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_not_contains "$rerun_commands" "tee -a" "reruns never append repository definitions"
assert_not_contains "$rerun_commands" ">>" "reruns never duplicate repository definitions"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_NVM_VERSION=v0.40.4
install_or_update_codex
install_or_update_nvm
install_or_update_node_tools
user_tool_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_contains "$user_tool_commands" "https://chatgpt.com/codex/install.sh" "Codex uses the official stable installer and updater"
assert_contains "$user_tool_commands" "https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.4/install.sh" "nvm installs the resolved stable release"
assert_contains "$user_tool_commands" "nvm install --lts --latest-npm" "Node converges on the latest LTS release"
assert_contains "$user_tool_commands" "nvm use --lts" "the current shell activates Node LTS"
assert_contains "$user_tool_commands" "npm install --global npm@latest" "npm upgrades to latest stable"
assert_contains "$user_tool_commands" "npm install --global corepack@latest" "Corepack upgrades to latest stable"
assert_contains "$user_tool_commands" "corepack enable" "Corepack shims are enabled"
assert_contains "$user_tool_commands" "corepack prepare pnpm@latest --activate" "pnpm activates its latest stable release"
assert_contains "$user_tool_commands" "corepack prepare yarn@stable --activate" "Yarn activates its stable release"

mkdir -p "$TEST_TMP/installer-tmp"
cat >"$TEST_TMP/success-installer.bash" <<'EOF'
#!/usr/bin/env bash
printf 'installer executed\n' >"$DEV_SETUP_INSTALLER_MARKER"
EOF
cat >"$TEST_TMP/failing-installer.bash" <<'EOF'
#!/usr/bin/env bash
exit 23
EOF
curl() {
  local output='' argument
  while (($# > 0)); do
    argument=$1
    shift
    if [[ "$argument" == -o ]]; then
      output=$1
      shift
    fi
  done
  cp "$DEV_SETUP_FIXTURE_INSTALLER" "$output"
}
export -f curl

export DEV_SETUP_INSTALLER_MARKER="$TEST_TMP/installer.marker"
export DEV_SETUP_FIXTURE_INSTALLER="$TEST_TMP/success-installer.bash"
DEV_SETUP_TEST_MODE=0 TMPDIR="$TEST_TMP/installer-tmp" "$BASH" -c \
  "source '$REPO_ROOT/linux/dev-server/setup.sh'; run_user_installer https://fixture.invalid/install bash"
assert_eq "installer executed" "$(<"$DEV_SETUP_INSTALLER_MARKER")" "downloaded installers are executed rather than only logged"

export DEV_SETUP_FIXTURE_INSTALLER="$TEST_TMP/failing-installer.bash"
set +e
DEV_SETUP_TEST_MODE=0 TMPDIR="$TEST_TMP/installer-tmp" "$BASH" -c \
  "source '$REPO_ROOT/linux/dev-server/setup.sh'; run_user_installer https://fixture.invalid/install bash"
installer_failure_status=$?
set -e
assert_eq 23 "$installer_failure_status" "installer failures propagate their original exit status"
installer_temp_count=$(find "$TEST_TMP/installer-tmp" -type f | wc -l | tr -d ' ')
assert_eq 0 "$installer_temp_count" "temporary installer files are removed after failures"

: >"$DEV_SETUP_COMMAND_LOG"
rm -rf "$DEV_SETUP_HOME/.bun"
install_or_update_bun
missing_bun_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_contains "$missing_bun_commands" "https://bun.sh/install" "missing Bun uses the official installer"

mkdir -p "$DEV_SETUP_HOME/.bun/bin"
: >"$DEV_SETUP_HOME/.bun/bin/bun"
chmod +x "$DEV_SETUP_HOME/.bun/bin/bun"
: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_BUN_PRESENT=1
install_or_update_bun
unset DEV_SETUP_BUN_PRESENT
existing_bun_commands=$(<"$DEV_SETUP_COMMAND_LOG")
assert_contains "$existing_bun_commands" "upgrade --stable" "existing Bun upgrades to stable"

: >"$DEV_SETUP_HOME/.bashrc"
ensure_shell_profile
ensure_shell_profile
profile_content=$(<"$DEV_SETUP_HOME/.bashrc")
assert_count 1 "$profile_content" "# >>> johanbostrom dev setup: nvm >>>" "reruns keep one nvm profile block"
assert_count 1 "$profile_content" "# >>> johanbostrom dev setup: bun >>>" "reruns keep one Bun profile block"

export XDG_CONFIG_HOME="$TEST_TMP/xdg"
load_nvm
assert_eq "$DEV_SETUP_HOME/.nvm" "$NVM_DIR" "nvm loads from the same directory used by its installer when XDG is set"
ensure_shell_profile
profile_content=$(<"$DEV_SETUP_HOME/.bashrc")
assert_contains "$profile_content" 'export NVM_DIR="$HOME/.nvm"' "future shells use the canonical nvm installation directory"
unset XDG_CONFIG_HOME

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_GIT_NAME="Existing User" DEV_SETUP_GIT_EMAIL="existing@example.com"
export DEV_SETUP_GH_AUTHENTICATED=0 DEV_SETUP_CODEX_AUTHENTICATED=0
export DEV_SETUP_DOCKER_MEMBER=0 DEV_SETUP_DOCKER_GROUP_EXISTS=1
run_wizard <<<"n"
assert_eq "" "$(<"$DEV_SETUP_COMMAND_LOG")" "declining the wizard performs no configuration"

: >"$DEV_SETUP_COMMAND_LOG"
run_wizard <<'EOF'
y
n
n
n
n
n
EOF
assert_eq "" "$(<"$DEV_SETUP_COMMAND_LOG")" "every wizard step can be skipped independently"
skipped_wizard_summary=$(print_summary)
assert_contains "$skipped_wizard_summary" "Git identity: skipped" "summary reports skipped Git identity setup"
assert_contains "$skipped_wizard_summary" "GitHub authentication: skipped" "summary reports skipped GitHub authentication"
assert_contains "$skipped_wizard_summary" "Codex authentication: skipped" "summary reports skipped Codex authentication"
assert_contains "$skipped_wizard_summary" "Docker access: skipped" "summary reports skipped Docker access"
assert_contains "$skipped_wizard_summary" "Node.js default: skipped" "summary reports skipped Node.js default setup"

: >"$DEV_SETUP_COMMAND_LOG"
git_identity_output=$(configure_git_identity <<<"n")
assert_contains "$git_identity_output" "Existing User" "Git setup displays the existing name"
assert_contains "$git_identity_output" "existing@example.com" "Git setup displays the existing email"
assert_not_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "git config --global user.name" "Git identity is preserved without confirmation"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_GH_AUTHENTICATED=0
configure_github_auth <<<"y"
assert_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "gh auth login" "GitHub authentication uses gh auth login"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_GH_AUTHENTICATED=1
configure_github_auth <<<"n"
assert_not_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "gh auth login" "existing GitHub authentication is preserved"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_CODEX_AUTHENTICATED=0
configure_codex_auth <<<"y"
assert_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "codex login" "Codex authentication uses codex login"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_CODEX_AUTHENTICATED=1
configure_codex_auth <<<"n"
assert_not_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "codex login" "existing Codex authentication is preserved"

: >"$DEV_SETUP_COMMAND_LOG"
export DEV_SETUP_DOCKER_MEMBER=0 DEV_SETUP_DOCKER_GROUP_EXISTS=1
docker_output=$(configure_docker_access <<<"y")
assert_contains "$docker_output" "docker group grants root-level privileges" "Docker setup discloses root-equivalent access"
assert_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "usermod -aG docker tester" "confirmed Docker setup adds only the invoking user"

: >"$DEV_SETUP_COMMAND_LOG"
DEV_SETUP_EFFECTIVE_UID=0
DEV_SETUP_USER=root
root_docker_output=$(configure_docker_access <<<"y")
assert_contains "$root_docker_output" "Docker access is already available" "root Docker access does not need group changes"
assert_not_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "usermod -aG docker" "root Docker setup does not modify group membership"
DEV_SETUP_EFFECTIVE_UID=$EUID
DEV_SETUP_USER=tester

: >"$DEV_SETUP_COMMAND_LOG"
configure_node_default <<<"y"
assert_contains "$(<"$DEV_SETUP_COMMAND_LOG")" "nvm alias default lts/\*" "confirmed Node setup selects LTS as nvm's default"

orchestration_output=$(
  require_supported_host() { :; }
  install_system_packages() { :; }
  install_user_tools() { :; }
  verify_tools() { :; }
  run_wizard() { :; }
  main
)
assert_in_order "$orchestration_output" "main runs all required phases in order" \
  "Checking host" \
  "Installing or updating system tools" \
  "Installing or updating user tools" \
  "Verifying installed tools" \
  "Optional setup wizard" \
  "Setup complete"

set +e
failure_output=$(
  {
    require_supported_host() { :; }
    install_system_packages() { return 42; }
    install_user_tools() { printf 'unexpected user tools\n'; }
    verify_tools() { printf 'unexpected verification\n'; }
    run_wizard() { printf 'unexpected wizard\n'; }
    main
  } 2>&1
)
failure_status=$?
set -e
if [[ $failure_status -ne 0 ]]; then
  pass "required phase failures propagate a nonzero status"
else
  fail "required phase failures propagate a nonzero status"
fi
assert_contains "$failure_output" "Installing or updating system tools failed" "failures name their phase"
assert_not_contains "$failure_output" "unexpected wizard" "failures stop before the wizard"
assert_not_contains "$failure_output" "Setup complete" "failures never print a success summary"

export DEV_SETUP_VERSION_GIT="git version 2.50.0"
export DEV_SETUP_VERSION_GH="gh version 2.75.0"
export DEV_SETUP_VERSION_DOCKER="Docker version 28.0.0"
export DEV_SETUP_VERSION_DOCKER_COMPOSE="Docker Compose version v2.38.0"
export DEV_SETUP_VERSION_NVM="0.40.4"
export DEV_SETUP_VERSION_NODE="v24.0.0"
export DEV_SETUP_VERSION_NPM="11.0.0"
export DEV_SETUP_VERSION_PNPM="10.0.0"
export DEV_SETUP_VERSION_YARN="4.9.0"
export DEV_SETUP_VERSION_BUN="1.2.0"
export DEV_SETUP_VERSION_CODEX="codex-cli 0.60.0"
set +e
version_output=$(verify_tools 2>&1)
version_status=$?
set -e
assert_eq 0 "$version_status" "version verification succeeds when every tool is available"
for tool_name in Git "GitHub CLI" Docker "Docker Compose" nvm Node.js npm pnpm Yarn Bun Codex; do
  assert_contains "$version_output" "$tool_name:" "version summary includes $tool_name"
done

DOCKER_LOGOUT_REQUIRED=1
summary_output=$(print_summary)
assert_count 1 "$summary_output" "Log out and back in before using Docker without sudo." "the logout note appears exactly once"

finish_tests
