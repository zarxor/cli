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

main() {
  require_supported_host
}

if [[ "${BASH_SOURCE[0]}" == "$0" && "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
  main "$@"
fi
