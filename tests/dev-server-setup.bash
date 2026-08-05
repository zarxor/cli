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
root_output=$(DEV_SETUP_EFFECTIVE_UID=0 DEV_SETUP_TEST_MODE=1 \
  "$BASH" -c "source '$REPO_ROOT/linux/dev-server/setup.sh'; require_non_root" 2>&1)
root_status=$?
set -e
if [[ $root_status -ne 0 ]]; then
  pass "rejects root execution"
else
  fail "rejects root execution"
fi
assert_contains "$root_output" "Run this script as a non-root user" "explains root rejection"

finish_tests
