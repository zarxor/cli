#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=dev
output_dir="${TMPDIR:-/tmp}/jb-release"
go_exe="$repo_root/.tools/go1.26.5/go/bin/go.exe"

usage() {
  printf 'Usage: %s [--version VERSION] [--output-dir DIRECTORY] [--go GO_EXECUTABLE]\n' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version=$2; shift 2 ;;
    --output-dir) output_dir=$2; shift 2 ;;
    --go) go_exe=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

[[ -x "$go_exe" ]] || { printf 'Verified Go executable not found: %s\n' "$go_exe" >&2; exit 1; }
mkdir -p "$output_dir"
staging_root=$(mktemp -d "${TMPDIR:-/tmp}/jb-build.XXXXXXXX")
trap 'rm -rf "$staging_root"' EXIT

build_target() {
  local os=$1 arch=$2 asset=$3 executable=$4 binary_path asset_path
  binary_path="$staging_root/$executable"
  asset_path="$output_dir/$asset"
  rm -f "$binary_path" "$asset_path" "$asset_path.sha256"
  (
    cd "$repo_root"
    GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 "$go_exe" build -trimpath -buildvcs=false \
      -ldflags "-s -w -X github.com/zarxor/scripts/internal/version.Version=$version" \
      -o "$binary_path" ./cmd/jb
  )
  if [[ $os == linux ]]; then
    (
      cd "$staging_root"
      tar --sort=name --mtime='@0' --owner=0 --group=0 --numeric-owner -cf - "$executable" | gzip -n >"$asset_path"
    )
  else
    (
      cd "$staging_root"
      touch -t 198001010000 "$executable"
      zip -X -q "$asset_path" "$executable"
    )
  fi
  (cd "$output_dir" && sha256sum "$asset" >"$asset.sha256")
  printf 'built %s\n' "$asset_path"
}

build_target linux amd64 jb_linux_amd64.tar.gz jb
build_target linux arm64 jb_linux_arm64.tar.gz jb
build_target windows amd64 jb_windows_amd64.zip jb.exe
build_target windows arm64 jb_windows_arm64.zip jb.exe
