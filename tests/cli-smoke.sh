#!/usr/bin/env bash
set -uo pipefail

TEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$TEST_DIR/.." && pwd)

# shellcheck source=tests/test_helper.bash
source "$TEST_DIR/test_helper.bash"

if [[ ! -f "$REPO_ROOT/install.sh" ]]; then
  fail "Linux bootstrap installer exists"
  finish_tests
  exit 1
fi

TEST_TMP=$(mktemp -d)
server_pid=
cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$TEST_TMP"
}
trap cleanup EXIT

release_dir=$TEST_TMP/release
payload_dir=$TEST_TMP/payload
shim_dir=$TEST_TMP/shims
mkdir -p "$release_dir" "$payload_dir" "$shim_dir"

cat >"$payload_dir/jb" <<'EOF'
#!/usr/bin/env bash
printf 'jb fixture linux amd64\n'
EOF
chmod +x "$payload_dir/jb"
tar -czf "$release_dir/jb_linux_amd64.tar.gz" -C "$payload_dir" jb
(
  cd "$release_dir" || exit 1
  sha256sum jb_linux_amd64.tar.gz >jb_linux_amd64.tar.gz.sha256
)

cat >"$shim_dir/uname" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  -s) printf 'Linux\n' ;;
  -m) printf 'x86_64\n' ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$shim_dir/uname"

port_file=$TEST_TMP/port
server_log=$TEST_TMP/server.log
python -c 'import functools,http.server,sys; handler=functools.partial(http.server.SimpleHTTPRequestHandler,directory=sys.argv[1]); server=http.server.ThreadingHTTPServer(("127.0.0.1",0),handler); open(sys.argv[2],"w",encoding="ascii").write(str(server.server_port)); server.serve_forever()' "$release_dir" "$port_file" >"$server_log" 2>&1 &
server_pid=$!

for _ in {1..100}; do
  [[ -s "$port_file" ]] && break
  sleep 0.05
done
if [[ ! -s "$port_file" ]]; then
  fail "fixture release server starts"
  finish_tests
  exit 1
fi
release_url="http://127.0.0.1:$(<"$port_file")"

install_dir=$TEST_TMP/home/.local/bin
set +e
install_output=$(HOME="$TEST_TMP/home" PATH="$shim_dir:$PATH" \
  JB_INSTALL_DIR="$install_dir" JB_RELEASE_BASE_URL="$release_url" \
  bash "$REPO_ROOT/install.sh" 2>&1)
install_status=$?
set -e
assert_eq 0 "$install_status" "Linux installer accepts a fixture release server"
if [[ -x "$install_dir/jb" ]]; then
  pass "Linux installer writes an executable to the selected user-local path"
  assert_eq "jb fixture linux amd64" "$($install_dir/jb)" "Linux installer extracts the selected amd64 binary"
else
  fail "Linux installer writes an executable to the selected user-local path"
fi
assert_contains "$install_output" "$install_dir/jb" "Linux installer reports the installed binary path"
assert_contains "$install_output" "$install_dir" "Linux installer prints the PATH action"
server_requests=$(<"$server_log")
assert_contains "$server_requests" 'GET /jb_linux_amd64.tar.gz ' "Linux installer selects the matching release asset"
assert_contains "$server_requests" 'GET /jb_linux_amd64.tar.gz.sha256 ' "Linux installer downloads the matching checksum"

printf '%064d  jb_linux_amd64.tar.gz\n' 0 >"$release_dir/jb_linux_amd64.tar.gz.sha256"
bad_install_dir=$TEST_TMP/bad/bin
set +e
bad_output=$(HOME="$TEST_TMP/home" PATH="$shim_dir:$PATH" \
  JB_INSTALL_DIR="$bad_install_dir" JB_RELEASE_BASE_URL="$release_url" \
  bash "$REPO_ROOT/install.sh" 2>&1)
bad_status=$?
set -e
if [[ $bad_status -ne 0 ]]; then
  pass "Linux installer rejects a checksum mismatch"
else
  fail "Linux installer rejects a checksum mismatch"
fi
if [[ ! -e "$bad_install_dir/jb" ]]; then
  pass "Linux installer does not install an unverified binary"
else
  fail "Linux installer does not install an unverified binary"
fi
assert_contains "${bad_output,,}" "checksum" "Linux checksum failures explain the rejection"

finish_tests
