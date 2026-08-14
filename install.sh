#!/usr/bin/env bash
set -Eeuo pipefail

RELEASE_BASE_URL=${JB_RELEASE_BASE_URL:-https://github.com/zarxor/cli/releases/latest/download}
INSTALL_DIR=${JB_INSTALL_DIR:-${HOME:?HOME is required}/.local/bin}
TEMP_DIR=

cleanup() {
  if [[ -n "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR"
  fi
}
trap cleanup EXIT INT TERM HUP

die() {
  printf '[jb installer] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required."
}

download() {
  local url=$1 destination=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$destination"
  else
    die "curl or wget is required."
  fi
}

detect_target() {
  local os architecture
  os=$(uname -s)
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "Unsupported operating system: $os" ;;
  esac

  architecture=$(uname -m)
  case "$architecture" in
    x86_64 | amd64) architecture=amd64 ;;
    aarch64 | arm64) architecture=arm64 ;;
    *) die "Unsupported architecture: $architecture" ;;
  esac

  printf '%s_%s' "$os" "$architecture"
}

sha256_file() {
  local file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    die "sha256sum or shasum is required to verify the release."
  fi
}

install_binary() {
  local source=$1 destination=$2
  if mkdir -p "$INSTALL_DIR" 2>/dev/null && [[ -w "$INSTALL_DIR" ]]; then
    install -m 0755 "$source" "$destination"
    return
  fi

  case "$INSTALL_DIR/" in
    /usr/local/* | /usr/* | /opt/*)
      require_command sudo
      sudo mkdir -p "$INSTALL_DIR"
      sudo install -m 0755 "$source" "$destination"
      ;;
    *)
      die "Cannot write to $INSTALL_DIR. Choose a user-writable JB_INSTALL_DIR."
      ;;
  esac
}

main() {
  local target asset archive checksum_file expected_checksum actual_checksum extracted destination
  require_command tar
  require_command install

  target=$(detect_target)
  asset="jb_${target}.tar.gz"
  TEMP_DIR=$(mktemp -d)
  archive=$TEMP_DIR/$asset
  checksum_file=$archive.sha256
  extracted=$TEMP_DIR/extracted
  destination=$INSTALL_DIR/jb

  printf '[jb installer] Downloading %s\n' "$asset"
  download "${RELEASE_BASE_URL%/}/$asset" "$archive"
  download "${RELEASE_BASE_URL%/}/$asset.sha256" "$checksum_file"

  expected_checksum=$(awk 'NR == 1 {print $1}' "$checksum_file")
  [[ "$expected_checksum" =~ ^[[:xdigit:]]{64}$ ]] || die "Invalid checksum file for $asset."
  actual_checksum=$(sha256_file "$archive")
  [[ "${actual_checksum,,}" == "${expected_checksum,,}" ]] || die "Checksum verification failed for $asset."
  printf '[jb installer] Checksum verified.\n'

  mkdir -p "$extracted"
  tar -xzf "$archive" -C "$extracted" jb
  [[ -f "$extracted/jb" ]] || die "$asset does not contain jb."
  install_binary "$extracted/jb" "$destination"

  printf '[jb installer] Installed %s\n' "$destination"
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) printf '[jb installer] jb is already on PATH.\n' ;;
    *) printf '[jb installer] Add it to this shell: export PATH="%s:$PATH"\n' "$INSTALL_DIR" ;;
  esac
}

main "$@"
