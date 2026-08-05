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

main() {
  require_supported_host
}

if [[ "${BASH_SOURCE[0]}" == "$0" && "${DEV_SETUP_TEST_MODE:-0}" != 1 ]]; then
  main "$@"
fi
