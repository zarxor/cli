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

  local ID= ID_LIKE= VERSION_CODENAME= UBUNTU_CODENAME= DEBIAN_CODENAME=
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

main() {
  require_supported_host
}

if [[ "${BASH_SOURCE[0]}" == "$0" && "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
  main "$@"
fi
