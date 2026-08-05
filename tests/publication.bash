#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"
cd "$REPO_ROOT"

script_mode=$(git ls-files --stage linux/dev-server/setup.sh | awk '{print $1}')
assert_eq 100755 "$script_mode" "the public setup script is executable in Git"

if [[ -f CNAME ]]; then
  assert_eq scripts.johanbostrom.se "$(<CNAME)" "CNAME declares the public scripts domain"
else
  fail "CNAME declares the public scripts domain"
fi

readme=$(<README.md)
assert_contains "$readme" "https://scripts.johanbostrom.se/linux/dev-server/setup.sh" "README indexes the permanent script URL"
assert_contains "$readme" "bash <(curl -fsSL https://scripts.johanbostrom.se/linux/dev-server/setup.sh)" "README preserves wizard input in its one-line command"
assert_contains "$readme" "Debian" "README documents Debian support"
assert_contains "$readme" "Arch" "README documents Arch support"
assert_contains "$readme" "rerun" "README documents update behavior on reruns"
assert_contains "$readme" "docker group grants root-level privileges" "README documents Docker's security impact"

if [[ -f .github/workflows/validate.yml ]]; then
  validation_workflow=$(<.github/workflows/validate.yml)
else
  validation_workflow=
fi
assert_contains "$validation_workflow" "actions/checkout@v6" "validation checks out the repository"
assert_contains "$validation_workflow" "shellcheck --severity=warning" "validation enforces ShellCheck warnings"
assert_contains "$validation_workflow" "bash tests/run.bash" "validation runs behavioral tests"

if [[ -f .github/workflows/pages.yml ]]; then
  pages_workflow=$(<.github/workflows/pages.yml)
else
  pages_workflow=
fi
assert_contains "$pages_workflow" "needs: validate" "Pages deployment cannot bypass validation"
assert_contains "$pages_workflow" "actions/configure-pages@v5" "Pages configures deployment metadata"
assert_contains "$pages_workflow" "actions/upload-pages-artifact@v4" "Pages uploads the static repository"
assert_contains "$pages_workflow" "actions/deploy-pages@v4" "Pages deploys through the supported action"

finish_tests
