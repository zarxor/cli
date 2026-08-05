#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"
cd "$REPO_ROOT" || exit 1

if [[ -f CNAME ]]; then
  assert_eq cli.johanbostrom.se "$(<CNAME)" "CNAME declares the CLI domain"
else
  fail "CNAME declares the CLI domain"
fi

for installer in install.sh install.ps1; do
  if [[ -f "$installer" ]]; then
    pass "$installer is published at the site root"
  else
    fail "$installer is published at the site root"
  fi
done

script_mode=$(git ls-files --stage install.sh | awk '{print $1}')
assert_eq 100755 "$script_mode" "the Linux bootstrap installer is executable in Git"

shell_eol=$(git check-attr eol -- install.sh | awk '{print $3}')
assert_eq lf "$shell_eol" "Git preserves LF line endings for the Linux bootstrap installer"

readme=$(<README.md)
assert_contains "$readme" "# Johan Bostrom CLI" "README uses the product name"
assert_contains "$readme" "https://cli.johanbostrom.se/install.sh" "README documents the Linux bootstrap URL"
assert_contains "$readme" "https://cli.johanbostrom.se/install.ps1" "README documents the Windows bootstrap URL"
assert_contains "$readme" "jb tools install --profiles=development" "README documents development profile installation"
assert_contains "$readme" 'jb tools install --profiles=<name>[,<name>...]' "README documents merged profile selection syntax"
assert_contains "$readme" "deduplicated" "README documents profile tool deduplication"
assert_contains "$readme" "jb tools update" "README documents live updates"
assert_contains "$readme" "discovers installed tools and versions live" "README explains stateless live update discovery"
assert_contains "$readme" "No local state database" "README explicitly documents stateless operation"
assert_contains "$readme" "jb version" "README documents the version command"
assert_contains "$readme" "Debian/Ubuntu" "README documents Debian and Ubuntu support"
assert_contains "$readme" "Arch Linux" "README documents Arch support"
assert_contains "$readme" "Windows" "README documents Windows support"
assert_contains "$readme" "macOS" "README identifies macOS as future support"
assert_contains "$readme" "GitHub Releases" "README explains binary publication"
assert_contains "$readme" "JB_RELEASE_BASE_URL" "README documents the release-server override"
assert_not_contains "$readme" "scripts.johanbostrom.se" "README removes the old public URL"
assert_not_contains "$readme" "linux/dev-server/setup.sh" "README removes the retired installer path"

finish_tests
