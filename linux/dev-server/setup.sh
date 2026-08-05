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
CURRENT_PHASE=initialization
DOCKER_LOGOUT_REQUIRED=0
WIZARD_STATUS=not-run

on_error() {
  local exit_code=$?
  printf '[dev-setup] ERROR: %s failed (exit %d).\n' "$CURRENT_PHASE" "$exit_code" >&2
  exit "$exit_code"
}

log() {
  printf '[dev-setup] %s\n' "$*"
}

die() {
  printf '[dev-setup] ERROR: %s\n' "$*" >&2
  return 1
}

record_command() {
  if [[ -n "${DEV_SETUP_COMMAND_LOG:-}" ]]; then
    printf '%q ' "$@" >>"$DEV_SETUP_COMMAND_LOG"
    printf '\n' >>"$DEV_SETUP_COMMAND_LOG"
  fi
}

run() {
  record_command "$@"

  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] || "$@"
}

install_root_file() {
  local source_url=$1 destination=$2 mode=${3:-0644} temporary_file
  record_command curl -fsSL "$source_url"
  record_command sudo install -m "$mode" downloaded-file "$destination"
  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] && return 0

  temporary_file=$(mktemp)
  if ! curl -fsSL "$source_url" -o "$temporary_file"; then
    rm -f "$temporary_file"
    die "Failed to download $source_url"
    return 1
  fi
  sudo install -m "$mode" "$temporary_file" "$destination"
  rm -f "$temporary_file"
}

write_root_file() {
  local destination=$1 content=$2
  record_command write-root-file "$destination" "$content"
  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] && return 0
  printf '%s\n' "$content" | sudo tee "$destination" >/dev/null
}

detect_platform() {
  [[ -r "$OS_RELEASE_FILE" ]] || die "Cannot read $OS_RELEASE_FILE"

  local ID='' ID_LIKE='' VERSION_CODENAME='' UBUNTU_CODENAME='' DEBIAN_CODENAME=''
  # shellcheck disable=SC1090
  source "$OS_RELEASE_FILE"

  DISTRO_ID=${ID:-}
  DISTRO_ID_LIKE=${ID_LIKE:-}
  DISTRO_VERSION_CODENAME=${VERSION_CODENAME:-}
  DISTRO_UBUNTU_CODENAME=${UBUNTU_CODENAME:-}
  DISTRO_DEBIAN_CODENAME=${DEBIAN_CODENAME:-}

  case " ${ID:-} ${ID_LIKE:-} " in
    *" arch "*) PLATFORM=arch ;;
    *" debian "* | *" ubuntu "*) PLATFORM=debian ;;
    *) die "Unsupported Linux distribution: ${ID:-unknown}" ;;
  esac
}

require_non_root() {
  [[ "$DEV_SETUP_EFFECTIVE_UID" -ne 0 ]] ||
    die "Run this script as a non-root user; the script will request sudo when needed."
}

require_supported_host() {
  [[ "$(uname -s)" == Linux ]] || die "Only Linux is supported."
  require_non_root
  command -v sudo >/dev/null || die "sudo is required."
  detect_platform
}

configure_github_cli_apt() {
  local architecture=${DEV_SETUP_DPKG_ARCH:-}
  if [[ -z "$architecture" ]]; then
    architecture=$(dpkg --print-architecture)
  fi

  run sudo install -m 0755 -d /etc/apt/keyrings /etc/apt/sources.list.d
  install_root_file \
    https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    /etc/apt/keyrings/githubcli-archive-keyring.gpg 0644
  write_root_file /etc/apt/sources.list.d/github-cli.list \
    "deb [arch=$architecture signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main"
}

configure_docker_apt() {
  local docker_apt_distro docker_apt_codename architecture=${DEV_SETUP_DPKG_ARCH:-}

  case " $DISTRO_ID $DISTRO_ID_LIKE " in
    *" ubuntu "*)
      docker_apt_distro=ubuntu
      docker_apt_codename=${DISTRO_UBUNTU_CODENAME:-$DISTRO_VERSION_CODENAME}
      ;;
    *" debian "*)
      docker_apt_distro=debian
      docker_apt_codename=${DISTRO_DEBIAN_CODENAME:-$DISTRO_VERSION_CODENAME}
      ;;
    *)
      die "Docker's official apt repository is not mapped for distribution '$DISTRO_ID'."
      return 1
      ;;
  esac

  [[ -n "$docker_apt_codename" ]] || {
    die "No compatible Docker apt codename is declared by '$DISTRO_ID'."
    return 1
  }
  if [[ -z "$architecture" ]]; then
    architecture=$(dpkg --print-architecture)
  fi

  run sudo install -m 0755 -d /etc/apt/keyrings /etc/apt/sources.list.d
  install_root_file \
    "https://download.docker.com/linux/$docker_apt_distro/gpg" \
    /etc/apt/keyrings/docker.asc 0644
  write_root_file /etc/apt/sources.list.d/docker.sources "Types: deb
URIs: https://download.docker.com/linux/$docker_apt_distro
Suites: $docker_apt_codename
Components: stable
Architectures: $architecture
Signed-By: /etc/apt/keyrings/docker.asc"
}

install_debian_packages() {
  run sudo apt-get update
  run sudo apt-get install -y ca-certificates curl git gnupg build-essential
  configure_github_cli_apt
  configure_docker_apt
  run sudo apt-get update
  run sudo apt-get install -y gh docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin
  run sudo systemctl enable --now docker.service
}

install_arch_packages() {
  run sudo pacman -Syu --noconfirm --needed \
    base-devel ca-certificates curl git github-cli docker docker-buildx docker-compose
  run sudo systemctl enable --now docker.service
}

install_system_packages() {
  case "$PLATFORM" in
    debian) install_debian_packages ;;
    arch) install_arch_packages ;;
    *) die "Platform detection must run before package installation." ;;
  esac
}

run_user_installer() {
  local source_url=$1 interpreter=$2 temporary_file
  record_command curl -fsSL "$source_url"
  record_command "$interpreter" downloaded-installer
  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] && return 0

  temporary_file=$(mktemp)
  if ! curl -fsSL "$source_url" -o "$temporary_file"; then
    rm -f "$temporary_file"
    die "Failed to download $source_url"
    return 1
  fi
  "$interpreter" "$temporary_file"
  rm -f "$temporary_file"
}

replace_profile_block() {
  local profile=$1 block_name=$2 block_content=$3
  local start_marker="# >>> johanbostrom dev setup: $block_name >>>"
  local end_marker="# <<< johanbostrom dev setup: $block_name <<<"
  local temporary_file

  mkdir -p "$(dirname "$profile")"
  touch "$profile"
  temporary_file=$(mktemp)
  awk -v start="$start_marker" -v end="$end_marker" '
    $0 == start { skipping = 1; next }
    $0 == end { skipping = 0; next }
    !skipping { print }
  ' "$profile" >"$temporary_file"
  {
    cat "$temporary_file"
    printf '\n%s\n%s\n%s\n' "$start_marker" "$block_content" "$end_marker"
  } >"$profile"
  rm -f "$temporary_file"
}

ensure_shell_profile() {
  local profile="$DEV_SETUP_HOME/.bashrc"
  replace_profile_block "$profile" paths \
    'export PATH="$HOME/.local/bin:$PATH"'
  replace_profile_block "$profile" nvm \
    'export NVM_DIR="${XDG_CONFIG_HOME:-$HOME/.nvm}"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"'
  replace_profile_block "$profile" bun \
    'export BUN_INSTALL="$HOME/.bun"
export PATH="$BUN_INSTALL/bin:$PATH"'
}

install_or_update_codex() {
  run_user_installer https://chatgpt.com/codex/install.sh sh
  export PATH="$DEV_SETUP_HOME/.local/bin:$PATH"
}

resolve_nvm_version() {
  local version=${DEV_SETUP_NVM_VERSION:-}
  if [[ -z "$version" ]]; then
    version=$(git ls-remote --tags --refs https://github.com/nvm-sh/nvm.git 'v*' |
      awk -F/ '{print $3}' | sort -V | tail -n1)
  fi
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    die "Could not resolve a stable nvm release."
    return 1
  }
  printf '%s\n' "$version"
}

install_or_update_nvm() {
  local nvm_version installer_url temporary_file
  nvm_version=$(resolve_nvm_version)
  installer_url="https://raw.githubusercontent.com/nvm-sh/nvm/$nvm_version/install.sh"
  record_command curl -fsSL "$installer_url"
  record_command env PROFILE=/dev/null "NVM_DIR=$DEV_SETUP_HOME/.nvm" bash downloaded-installer
  [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]] && return 0

  temporary_file=$(mktemp)
  if ! curl -fsSL "$installer_url" -o "$temporary_file"; then
    rm -f "$temporary_file"
    die "Failed to download $installer_url"
    return 1
  fi
  PROFILE=/dev/null NVM_DIR="$DEV_SETUP_HOME/.nvm" bash "$temporary_file"
  rm -f "$temporary_file"
}

load_nvm() {
  export NVM_DIR=${XDG_CONFIG_HOME:-$DEV_SETUP_HOME/.nvm}
  if [[ "${DEV_SETUP_TEST_MODE:-0}" == 1 ]]; then
    return 0
  fi
  [[ -s "$NVM_DIR/nvm.sh" ]] || {
    die "nvm was installed but $NVM_DIR/nvm.sh is missing."
    return 1
  }
  # shellcheck disable=SC1090
  source "$NVM_DIR/nvm.sh"
}

install_or_update_node_tools() {
  load_nvm
  run nvm install --lts --latest-npm
  run nvm use --lts
  run npm install --global npm@latest
  run npm install --global corepack@latest
  run corepack enable
  run corepack prepare pnpm@latest --activate
  run corepack prepare yarn@stable --activate
}

install_or_update_bun() {
  local bun_bin="$DEV_SETUP_HOME/.bun/bin/bun" temporary_file
  if [[ "${DEV_SETUP_BUN_PRESENT:-0}" == 1 || -x "$bun_bin" ]]; then
    run "$bun_bin" upgrade --stable
  else
    record_command curl -fsSL https://bun.sh/install
    record_command env "BUN_INSTALL=$DEV_SETUP_HOME/.bun" bash downloaded-installer
    if [[ "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
      temporary_file=$(mktemp)
      if ! curl -fsSL https://bun.sh/install -o "$temporary_file"; then
        rm -f "$temporary_file"
        die "Failed to download https://bun.sh/install"
        return 1
      fi
      BUN_INSTALL="$DEV_SETUP_HOME/.bun" bash "$temporary_file"
      rm -f "$temporary_file"
    fi
  fi
  export BUN_INSTALL="$DEV_SETUP_HOME/.bun"
  export PATH="$BUN_INSTALL/bin:$PATH"
}

install_user_tools() {
  install_or_update_codex
  install_or_update_nvm
  install_or_update_node_tools
  install_or_update_bun
  ensure_shell_profile
}

confirm() {
  local prompt=$1 answer
  printf '%s [y/N] ' "$prompt"
  IFS= read -r answer || return 1
  [[ "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}

configure_git_identity() {
  local current_name current_email new_name new_email
  current_name=${DEV_SETUP_GIT_NAME:-$(git config --global --get user.name 2>/dev/null || true)}
  current_email=${DEV_SETUP_GIT_EMAIL:-$(git config --global --get user.email 2>/dev/null || true)}
  printf 'Current Git name: %s\n' "${current_name:-not set}"
  printf 'Current Git email: %s\n' "${current_email:-not set}"

  if ! confirm "Configure Git name and email?"; then
    return 0
  fi

  printf 'Git name: '
  IFS= read -r new_name
  printf 'Git email: '
  IFS= read -r new_email
  [[ -n "$new_name" && -n "$new_email" ]] || {
    log "Git identity skipped because both values are required."
    return 0
  }
  printf 'Proposed Git identity: %s <%s>\n' "$new_name" "$new_email"
  if confirm "Save this Git identity?"; then
    run git config --global user.name "$new_name"
    run git config --global user.email "$new_email"
  fi
}

github_is_authenticated() {
  if [[ -n "${DEV_SETUP_GH_AUTHENTICATED:-}" ]]; then
    [[ "$DEV_SETUP_GH_AUTHENTICATED" == 1 ]]
  else
    gh auth status >/dev/null 2>&1
  fi
}

configure_github_auth() {
  if github_is_authenticated; then
    log "GitHub CLI is already authenticated."
    confirm "Reauthenticate GitHub CLI?" && run gh auth login
  else
    confirm "Authenticate GitHub CLI?" && run gh auth login
  fi
  return 0
}

codex_is_authenticated() {
  if [[ -n "${DEV_SETUP_CODEX_AUTHENTICATED:-}" ]]; then
    [[ "$DEV_SETUP_CODEX_AUTHENTICATED" == 1 ]]
  else
    codex login status >/dev/null 2>&1
  fi
}

configure_codex_auth() {
  if codex_is_authenticated; then
    log "Codex is already authenticated."
    confirm "Reauthenticate Codex?" && run codex login
  else
    confirm "Authenticate Codex?" && run codex login
  fi
  return 0
}

docker_user_is_member() {
  if [[ -n "${DEV_SETUP_DOCKER_MEMBER:-}" ]]; then
    [[ "$DEV_SETUP_DOCKER_MEMBER" == 1 ]]
  else
    id -nG "$DEV_SETUP_USER" 2>/dev/null | tr ' ' '\n' | grep -Fxq docker
  fi
}

docker_group_exists() {
  if [[ -n "${DEV_SETUP_DOCKER_GROUP_EXISTS:-}" ]]; then
    [[ "$DEV_SETUP_DOCKER_GROUP_EXISTS" == 1 ]]
  else
    getent group docker >/dev/null 2>&1
  fi
}

configure_docker_access() {
  if docker_user_is_member; then
    log "$DEV_SETUP_USER already belongs to the docker group."
    return 0
  fi

  printf 'Membership in the docker group grants root-level privileges.\n'
  if confirm "Allow $DEV_SETUP_USER to run Docker without sudo?"; then
    docker_group_exists || run sudo groupadd docker
    run sudo usermod -aG docker "$DEV_SETUP_USER"
    DOCKER_LOGOUT_REQUIRED=1
  fi
}

configure_node_default() {
  local active_version=${DEV_SETUP_NODE_VERSION:-latest-LTS}
  if [[ "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
    active_version=$(nvm current)
  fi
  printf 'Active Node.js LTS: %s\n' "$active_version"
  confirm "Use the latest Node.js LTS as nvm's default?" &&
    run nvm alias default 'lts/*'
  return 0
}

run_wizard() {
  if ! confirm "Installation finished. Start the optional setup wizard?"; then
    WIZARD_STATUS=skipped
    log "Setup wizard skipped."
    return 0
  fi

  configure_git_identity
  configure_github_auth
  configure_codex_auth
  configure_docker_access
  configure_node_default
  WIZARD_STATUS=completed
}

read_version() {
  local label=$1 override_name=$2
  shift 2
  local version=${!override_name:-}
  if [[ -z "$version" ]]; then
    version=$("$@" 2>&1 | head -n1)
  fi
  [[ -n "$version" ]] || {
    die "$label is not available after installation."
    return 1
  }
  printf '%s: %s\n' "$label" "$version"
}

verify_tools() {
  read_version Git DEV_SETUP_VERSION_GIT git --version
  read_version "GitHub CLI" DEV_SETUP_VERSION_GH gh --version
  read_version Docker DEV_SETUP_VERSION_DOCKER docker --version
  read_version "Docker Compose" DEV_SETUP_VERSION_DOCKER_COMPOSE docker compose version
  read_version nvm DEV_SETUP_VERSION_NVM nvm --version
  read_version Node.js DEV_SETUP_VERSION_NODE node --version
  read_version npm DEV_SETUP_VERSION_NPM npm --version
  read_version pnpm DEV_SETUP_VERSION_PNPM pnpm --version
  read_version Yarn DEV_SETUP_VERSION_YARN yarn --version
  read_version Bun DEV_SETUP_VERSION_BUN bun --version
  read_version Codex DEV_SETUP_VERSION_CODEX codex --version
}

print_summary() {
  printf '[dev-setup] Wizard: %s.\n' "$WIZARD_STATUS"
  if [[ "$DOCKER_LOGOUT_REQUIRED" == 1 ]]; then
    printf '[dev-setup] Log out and back in before using Docker without sudo.\n'
  fi
  printf '[dev-setup] Open a new shell to load any updated PATH entries.\n'
}

main() {
  trap on_error ERR
  CURRENT_PHASE="Checking host"
  log "$CURRENT_PHASE"
  require_supported_host

  CURRENT_PHASE="Installing or updating system tools"
  log "$CURRENT_PHASE"
  install_system_packages

  CURRENT_PHASE="Installing or updating user tools"
  log "$CURRENT_PHASE"
  install_user_tools

  CURRENT_PHASE="Verifying installed tools"
  log "$CURRENT_PHASE"
  verify_tools

  CURRENT_PHASE="Optional setup wizard"
  log "$CURRENT_PHASE"
  run_wizard

  print_summary
  CURRENT_PHASE="Setup complete"
  log "$CURRENT_PHASE"
}

if [[ "${BASH_SOURCE[0]}" == "$0" && "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
  main "$@"
fi
