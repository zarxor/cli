#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
status=0

for test_file in "$TEST_DIR/dev-server-setup.bash" "$TEST_DIR/publication.bash"; do
  "$BASH" "$test_file" || status=1
done

exit "$status"
