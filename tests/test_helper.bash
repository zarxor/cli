#!/usr/bin/env bash

TEST_FAILURES=0

fail() {
  printf 'not ok - %s\n' "$1" >&2
  TEST_FAILURES=$((TEST_FAILURES + 1))
}

pass() {
  printf 'ok - %s\n' "$1"
}

assert_eq() {
  local expected=$1 actual=$2 message=$3
  if [[ "$actual" == "$expected" ]]; then
    pass "$message"
  else
    fail "$message (expected '$expected', got '$actual')"
  fi
}

assert_contains() {
  local haystack=$1 needle=$2 message=$3
  if [[ "$haystack" == *"$needle"* ]]; then
    pass "$message"
  else
    fail "$message (missing '$needle')"
  fi
}

assert_not_contains() {
  local haystack=$1 needle=$2 message=$3
  if [[ "$haystack" != *"$needle"* ]]; then
    pass "$message"
  else
    fail "$message (unexpected '$needle')"
  fi
}

assert_count() {
  local expected=$1 haystack=$2 needle=$3 message=$4 count remainder
  count=0
  remainder=$haystack
  while [[ "$remainder" == *"$needle"* ]]; do
    remainder=${remainder#*"$needle"}
    count=$((count + 1))
  done
  assert_eq "$expected" "$count" "$message"
}

assert_in_order() {
  local haystack=$1 message=$2
  shift 2
  local needle remainder=$haystack
  for needle in "$@"; do
    if [[ "$remainder" != *"$needle"* ]]; then
      fail "$message (missing or out of order: '$needle')"
      return
    fi
    remainder=${remainder#*"$needle"}
  done
  pass "$message"
}

finish_tests() {
  if ((TEST_FAILURES > 0)); then
    printf '%d test(s) failed\n' "$TEST_FAILURES" >&2
    return 1
  fi
}
