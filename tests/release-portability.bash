#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"

test_root=$(mktemp -d "${TMPDIR:-/tmp}/jb-release-portability.XXXXXXXX")
trap 'rm -rf "$test_root"' EXIT
real_bash=$(command -v bash)

wrap_command() {
  local name=$1 source
  source=$(command -v "$name")
  printf '%s\n' '#!/bin/bash' "exec $source \"\$@\"" >"$test_root/bin/$name"
  chmod +x "$test_root/bin/$name"
}

make_fake_bin() {
  rm -rf "${test_root:?}/bin"
  mkdir -p "$test_root/bin"
  for name in dirname mkdir mktemp rm touch; do
    wrap_command "$name"
  done
}

write_fake() {
  local name=$1
  shift
  printf '%s\n' '#!/bin/bash' "$@" >"$test_root/bin/$name"
  chmod +x "$test_root/bin/$name"
}

make_fake_bin
tool_log="$test_root/build-tools.log"
write_fake uname 'printf "Darwin\n"'
write_fake go \
  'output=' \
  'while (($#)); do' \
  '  if [[ $1 == -o ]]; then shift; output=$1; fi' \
  '  shift' \
  'done' \
  'printf binary >"$output"'
write_fake gtar 'printf "gtar %s\n" "$*" >>"$JB_TEST_TOOL_LOG"; printf archive'
write_fake gzip 'printf gzip'
write_fake zip 'printf "zip %s\n" "$*" >>"$JB_TEST_TOOL_LOG"; printf archive >"$3"'
write_fake shasum 'printf "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  %s\n" "${*: -1}"'

build_dir="$test_root/build"
build_output=$(PATH="$test_root/bin" JB_TEST_TOOL_LOG="$tool_log" "$real_bash" \
  "$REPO_ROOT/scripts/build-local.sh" --version v1.2.3 --output-dir "$build_dir" --go "$test_root/bin/go" 2>&1)
build_status=$?
if ((build_status != 0)); then
  printf 'build output: %s\n' "$build_output" >&2
fi
assert_eq 0 "$build_status" "Bash artifact builder works with macOS tool names"
if [[ -f $tool_log ]]; then
  build_log=$(<"$tool_log")
else
  build_log=
fi
assert_contains "$build_log" "gtar" "Bash artifact builder uses GNU tar as gtar"
for asset in \
  jb_linux_amd64.tar.gz jb_linux_arm64.tar.gz \
  jb_windows_amd64.zip jb_windows_arm64.zip; do
  if [[ -f "$build_dir/$asset" && -f "$build_dir/$asset.sha256" ]]; then
    pass "Bash artifact builder emits $asset and its checksum"
  else
    fail "Bash artifact builder emits $asset and its checksum"
  fi
done

make_fake_bin
write_fake uname 'if [[ $1 == -m ]]; then printf "arm64\n"; else printf "Darwin\n"; fi'
write_fake awk \
  'if [[ $1 == *NR* ]]; then' \
  '  printf "1\n"' \
  'else' \
  '  read -r first _' \
  '  printf "%s\n" "$first"' \
  'fi'
write_fake shasum 'printf "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  %s\n" "${*: -1}"'
write_fake tar 'printf "tar %s\n" "$*" >>"$JB_TEST_TOOL_LOG"; printf "jb\n"'
write_fake unzip 'printf "unzip %s\n" "$*" >>"$JB_TEST_TOOL_LOG"; printf "jb.exe\n"'

check_dir="$test_root/check"
mkdir -p "$check_dir"
for asset in \
  jb_linux_amd64.tar.gz jb_linux_arm64.tar.gz \
  jb_windows_amd64.zip jb_windows_arm64.zip; do
  printf archive >"$check_dir/$asset"
  printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  %s\n' "$asset" >"$check_dir/$asset.sha256"
done
tool_log="$test_root/check-tools.log"
check_output=$(PATH="$test_root/bin" JB_TEST_TOOL_LOG="$tool_log" "$real_bash" \
  "$REPO_ROOT/scripts/check-artifacts.sh" --version v1.2.3 --artifact-dir "$check_dir" 2>&1)
check_status=$?
if ((check_status != 0)); then
  printf 'check output: %s\n' "$check_output" >&2
fi
assert_eq 0 "$check_status" "Bash artifact checker supports shasum on macOS"
if [[ -f $tool_log ]]; then
  check_log=$(<"$tool_log")
else
  check_log=
fi
assert_not_contains "$check_log" "-xzf" "macOS checker does not execute a Linux artifact"
assert_not_contains "$check_log" "-d " "macOS checker does not execute a Windows artifact"

make_fake_bin
write_fake uname 'printf "Darwin\n"'
write_fake go 'exit 0'
write_fake shasum 'exit 0'
write_fake tar 'printf "bsdtar 3.5.0\n"'
missing_gtar_output=$(PATH="$test_root/bin" "$real_bash" \
  "$REPO_ROOT/scripts/build-local.sh" --output-dir "$test_root/missing-gtar" --go "$test_root/bin/go" 2>&1)
missing_gtar_status=$?
if ((missing_gtar_status == 127)); then
  printf 'missing-gtar output: %s\n' "$missing_gtar_output" >&2
fi
if ((missing_gtar_status != 0)); then
  pass "Bash artifact builder rejects non-GNU tar"
else
  fail "Bash artifact builder rejects non-GNU tar"
fi
assert_contains "$missing_gtar_output" "brew install gnu-tar" "Bash artifact builder explains the macOS GNU tar prerequisite"

finish_tests
