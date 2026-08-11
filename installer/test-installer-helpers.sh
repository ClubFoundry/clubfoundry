#!/usr/bin/env bash
set -euo pipefail

INSTALLER="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/install.sh"
TEST_ROOT="$(mktemp -d /tmp/clubfoundry-installer-helpers.XXXXXX)"
CURL_CALLS_FILE="$TEST_ROOT/curl.calls"
trap 'rm -rf -- "$TEST_ROOT"' EXIT

extract_function() {
  local name="$1"
  awk -v function_name="$name" '
    $0 ~ "^" function_name "\\(\\) \\{" { capture = 1 }
    capture { print }
    capture && $0 == "}" { exit }
  ' "$INSTALLER"
}

load_function() {
  local name="$1"
  local source
  source=$(extract_function "$name")
  [ -n "$source" ] || {
    echo "function not found: $name" >&2
    exit 1
  }
  eval "$source"
}

expect_equal() {
  local description="$1"
  local expected="$2"
  local actual="$3"
  if [ "$actual" != "$expected" ]; then
    printf '%s: expected <%s>, got <%s>\n' "$description" "$expected" "$actual" >&2
    exit 1
  fi
}

expect_success() {
  local description="$1"
  shift
  if ! "$@"; then
    echo "$description: expected success" >&2
    exit 1
  fi
}

expect_failure() {
  local description="$1"
  shift
  if "$@"; then
    echo "$description: expected failure" >&2
    exit 1
  fi
}

load_function fmt_elapsed
expect_equal "zero milliseconds" "1s" "$(fmt_elapsed 0)"
expect_equal "sub-second duration" "1s" "$(fmt_elapsed 999)"
expect_equal "one second" "1s" "$(fmt_elapsed 1000)"
expect_equal "round up partial second" "2s" "$(fmt_elapsed 1001)"
expect_equal "one minute boundary" "1m 0s" "$(fmt_elapsed 60000)"
expect_equal "minute plus second" "1m 1s" "$(fmt_elapsed 61000)"
expect_equal "duration above one hour" "60m 0s" "$(fmt_elapsed 3600000)"

load_function is_our_image
for image in \
  clubfoundry:1.3.143 \
  clubfoundry-updater:v3.AL \
  ghcr.io/clubfoundry/clubfoundry:1.3.143 \
  registry.example/namespace/clubfoundry-updater:v3.AL; do
  expect_success "owned image $image" is_our_image "$image"
done
for image in \
  clubfoundry \
  postgres:16 \
  ghcr.io/clubfoundry/not-clubfoundry:1.0; do
  expect_failure "foreign image $image" is_our_image "$image"
done

SS_FIXTURE=""
SS_STATUS=0
ss() {
  printf '%s' "$SS_FIXTURE"
  return "$SS_STATUS"
}

load_function port_free
SS_FIXTURE=""
expect_success "empty listener list" port_free 3000
SS_FIXTURE=$'LISTEN 0 4096 0.0.0.0:3000 0.0.0.0:*\n'
expect_failure "IPv4 listener on requested port" port_free 3000
SS_FIXTURE=$'LISTEN 0 4096 [::]:3000 [::]:*\n'
expect_failure "IPv6 listener on requested port" port_free 3000
SS_FIXTURE=$'LISTEN 0 4096 127.0.0.1:13000 0.0.0.0:*\n'
expect_success "different numeric suffix" port_free 3000
SS_FIXTURE=""
SS_STATUS=1
expect_success "unavailable listener probe" port_free 3000
SS_STATUS=0

MIRROR_LINES=""
jq() {
  cat >/dev/null
  printf '%s' "$MIRROR_LINES"
}
curl() {
  local argument
  local url=""
  for argument in "$@"; do
    case "$argument" in
      http://* | https://*) url="$argument" ;;
    esac
  done
  printf '%s\n' "$url" >>"$CURL_CALLS_FILE"
  [ "$url" = "https://fast.example/main.tar.gz" ]
}

load_function pick_fastest_mirror
MIRROR_LINES=$'https://slow.example/main.tar.gz\nhttps://fast.example/main.tar.gz\n'
: >"$CURL_CALLS_FILE"
expect_equal \
  "first reachable mirror" \
  "https://fast.example/main.tar.gz" \
  "$(pick_fastest_mirror '[fixture]' 'https://legacy.example/main.tar.gz')"
expect_equal \
  "mirror probe order" \
  $'https://slow.example/main.tar.gz\nhttps://fast.example/main.tar.gz' \
  "$(<"$CURL_CALLS_FILE")"

MIRROR_LINES=""
: >"$CURL_CALLS_FILE"
expect_equal \
  "empty mirror list fallback" \
  "https://legacy.example/main.tar.gz" \
  "$(pick_fastest_mirror '[]' 'https://legacy.example/main.tar.gz')"
expect_equal "empty mirror list skips probes" "" "$(<"$CURL_CALLS_FILE")"

MIRROR_LINES=$'https://slow.example/main.tar.gz\n'
: >"$CURL_CALLS_FILE"
expect_equal \
  "unreachable mirror fallback" \
  "https://legacy.example/main.tar.gz" \
  "$(pick_fastest_mirror '[fixture]' 'https://legacy.example/main.tar.gz')"

echo "installer helper contracts: OK"
