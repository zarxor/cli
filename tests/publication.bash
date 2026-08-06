#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"
cd "$REPO_ROOT" || exit 1

if [[ -f CNAME ]]; then
  cname=$(tr -d '\r\n' <CNAME)
  assert_eq cli.johanbostrom.se "$cname" "CNAME declares the CLI domain"
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

git_bin=git
if command -v git.exe >/dev/null 2>&1; then
  git_bin=git.exe
fi

script_mode=$($git_bin ls-files --stage install.sh | awk '{print $1}')
assert_eq 100755 "$script_mode" "the Linux bootstrap installer is executable in Git"

shell_eol=$($git_bin check-attr eol -- install.sh | awk '{print $3}')
assert_eq lf "$shell_eol" "Git preserves LF line endings for the Linux bootstrap installer"

readme=$(<README.md)
readme=${readme//$'\r'/}
assert_contains "$readme" "# Johan Bostrom CLI" "README uses the product name"
assert_contains "$readme" "https://cli.johanbostrom.se/install.sh" "README documents the Linux bootstrap URL"
assert_contains "$readme" "https://cli.johanbostrom.se/install.ps1" "README documents the Windows bootstrap URL"
assert_contains "$readme" $'```text\njb tools install\n```' "README documents full catalog installation"
assert_contains "$readme" "jb tools install --yes" "README documents non-interactive full catalog installation"
assert_contains "$readme" "preselected" "README explains the default interactive selection"
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

for site_file in site/index.html site/styles.css; do
  if [[ -f "$site_file" ]]; then
    pass "$site_file is present in the published site source"
  else
    fail "$site_file is present in the published site source"
  fi
done

site_page=""
if [[ -f site/index.html ]]; then
  site_page=$(<site/index.html)
fi

assert_contains "$site_page" 'href="styles.css"' "site page loads its local stylesheet"
assert_contains "$site_page" "Johan Bostrom CLI" "site page uses the product name"
assert_contains "$site_page" 'href="/install.sh"' "site page links to the Linux installer"
assert_contains "$site_page" 'href="/install.ps1"' "site page links to the Windows installer"
assert_contains "$site_page" "Debian/Ubuntu" "site page names Debian and Ubuntu support"
assert_contains "$site_page" "Arch Linux" "site page names Arch support"
assert_contains "$site_page" "Windows" "site page names Windows support"
assert_contains "$site_page" "macOS" "site page identifies macOS support status"
assert_contains "$site_page" '<code>jb tools install</code>' "site page documents full catalog installation"
assert_contains "$site_page" '<code>jb tools install --yes</code>' "site page documents non-interactive full catalog installation"
assert_contains "$site_page" "preselected" "site page explains the default interactive selection"
assert_contains "$site_page" "jb tools install --profiles=development" "site page documents profile installation"
assert_contains "$site_page" "jb tools install --profiles=development --only=bun" "site page documents narrowed profile installation"
assert_contains "$site_page" "jb tools update" "site page documents live updates"
assert_contains "$site_page" "jb tools update --profiles=development --only=bun" "site page documents narrowed profile updates"
assert_contains "$site_page" "Narrow the profile to Bun and its required dependencies; only installed members are updated." "site page retains dependencies for narrowed profile updates"
assert_contains "$site_page" "https://github.com/zarxor/scripts" "site page links to the repository"
assert_contains "$site_page" "https://github.com/zarxor/scripts/releases" "site page links to Releases"
assert_not_contains "$site_page" "scripts.johanbostrom.se" "site page removes the old public URL"
assert_not_contains "$site_page" "linux/dev-server/setup.sh" "site page removes the retired installer path"

for workflow_path in .github/workflows/validate.yml .github/workflows/pages.yml; do
  workflow=$(<"$workflow_path")
  workflow_name=${workflow_path##*/}
  assert_contains "$workflow" "bash -n install.sh tests/cli-smoke.sh tests/*.bash" "$workflow_name validates every published Bash entry point"
  assert_contains "$workflow" "shellcheck --severity=warning install.sh tests/cli-smoke.sh tests/*.bash" "$workflow_name ShellChecks every published Bash entry point"
  assert_contains "$workflow" "python" "$workflow_name installs the fixture release server dependency"
  assert_contains "$workflow" "curl" "$workflow_name installs the bootstrap download dependency"
  assert_contains "$workflow" "bash tests/run.bash" "$workflow_name runs smoke and publication tests"
  assert_not_contains "$workflow" "linux/dev-server/setup.sh" "$workflow_name removes the retired installer path"
done

pages_workflow=$(<.github/workflows/pages.yml)
assert_contains "$pages_workflow" "- name: Assemble Pages site" "Pages workflow assembles a dedicated site artifact"
assert_contains "$pages_workflow" "rm -rf _site" "Pages workflow clears the dedicated site artifact"
assert_contains "$pages_workflow" "mkdir -p _site" "Pages workflow creates the dedicated site artifact"
assert_contains "$pages_workflow" "cp -R site/. _site/" "Pages workflow copies the published site files"
assert_contains "$pages_workflow" "cp install.sh install.ps1 CNAME _site/" "Pages workflow copies published installers and CNAME"
assert_contains "$pages_workflow" "path: _site" "Pages workflow uploads the dedicated site artifact"
assert_not_contains "$pages_workflow" "path: ." "Pages workflow does not upload the repository root"

finish_tests
