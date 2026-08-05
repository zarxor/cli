#!/usr/bin/env bash
set -euo pipefail

version=dev
artifact_dir="${TMPDIR:-/tmp}/jb-release"

usage() {
  printf 'Usage: %s [--version VERSION] [--artifact-dir DIRECTORY]\n' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version=$2; shift 2 ;;
    --artifact-dir) artifact_dir=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

host_os=linux
[[ $(uname -s) == MINGW* || $(uname -s) == MSYS* || $(uname -s) == CYGWIN* ]] && host_os=windows
case $(uname -m) in
  x86_64|amd64) host_arch=amd64 ;;
  aarch64|arm64) host_arch=arm64 ;;
  *) host_arch=unsupported ;;
esac

check_target() {
  local os=$1 arch=$2 asset=$3 executable=$4 asset_path checksum members extract_root output
  asset_path="$artifact_dir/$asset"
  [[ -f "$asset_path" ]] || { printf 'Missing release asset: %s\n' "$asset_path" >&2; return 1; }
  checksum="$asset_path.sha256"
  [[ -f "$checksum" ]] || { printf 'Missing checksum: %s\n' "$checksum" >&2; return 1; }
  (cd "$artifact_dir" && sha256sum --strict --check "$(basename "$checksum")")
  if [[ $os == linux ]]; then
    members=$(tar -tzf "$asset_path")
  else
    members=$(unzip -Z1 "$asset_path")
  fi
  [[ $members == "$executable" ]] || { printf 'Unexpected archive members in %s: %s\n' "$asset" "$members" >&2; return 1; }

  if [[ $os == "$host_os" && $arch == "$host_arch" ]]; then
    extract_root=$(mktemp -d "${TMPDIR:-/tmp}/jb-check.XXXXXXXX")
    if [[ $os == linux ]]; then
      tar -xzf "$asset_path" -C "$extract_root"
    else
      unzip -q "$asset_path" -d "$extract_root"
    fi
    output=$("$extract_root/$executable" version)
    rm -rf "$extract_root"
    [[ $output == "Johan Bostrom CLI $version" || $output == "Johan Bostrom CLI $version\\n" ]] || { printf 'Unexpected version output from %s: %s\n' "$asset" "$output" >&2; return 1; }
  fi
  printf 'ok - %s\n' "$asset"
}

check_target linux amd64 jb_linux_amd64.tar.gz jb
check_target linux arm64 jb_linux_arm64.tar.gz jb
check_target windows amd64 jb_windows_amd64.zip jb.exe
check_target windows arm64 jb_windows_arm64.zip jb.exe
