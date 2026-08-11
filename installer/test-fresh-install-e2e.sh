#!/bin/bash
# Fresh-install E2E test for installer/install.sh.
#
# Purpose: prove a clean TrueNAS SCALE host can go from zero to a working
# ClubFoundry stack via the installer, without hand-holding. It also verifies
# that the unprivileged app container can write its generated secrets.
#
# Requires: an explicitly authorized disposable TrueNAS SCALE host, test images,
# and a patched installer copy that uses the isolated Compose project, container
# names, and updater port below. Set EXPECTED_MAIN_IMAGE to the exact main image
# used by that copy.
#
# Runs a SED-PATCHED COPY of install.sh against an isolated target:
#   /tmp/fresh-install-test/{app,app/data}   (never touches /opt/clubfoundry)
#   clubfoundry-fresh-test, clubfoundry-updater-fresh-test  (distinct names)
#   ports 3004 + 3005                        (no conflict with real main)
#
# On success, verifies 9 invariants. Tears down everything (containers,
# dirs, volumes) before exiting. Idempotent — re-run cleans leftovers first.
#
# Invocation (from the disposable test host):
#   export EXPECTED_MAIN_IMAGE=registry.example/clubfoundry:test
#   bash /tmp/fresh-install-test/run-test.sh
#
# Preparation (one-time per test run):
#   copy installer/install.sh to /tmp/fresh-install-test/install-under-test.sh
#   then patch its image, Compose project, container, and updater-port defaults
#   to the fixtures below. The project name must be isolated as well as the
#   container names so Compose cannot adopt the primary installation.

set -u

CONT="clubfoundry-fresh-test"
UPD="clubfoundry-updater-fresh-test"
APP_DIR="/tmp/fresh-install-test/app"
DATA_DIR="/tmp/fresh-install-test/app/data"
PORT=3004
UPD_PORT=3005
EXPECTED_MAIN_IMAGE="${EXPECTED_MAIN_IMAGE:-}"
INSTALLER_UNDER_TEST="/tmp/fresh-install-test/install-under-test.sh"

if [ -z "$EXPECTED_MAIN_IMAGE" ]; then
  echo "[ERROR] EXPECTED_MAIN_IMAGE must name the exact test image"
  exit 1
fi

# Fail before any Docker mutation if the prepared installer could share Compose
# ownership labels or container names with the primary installation.
if [ ! -f "$INSTALLER_UNDER_TEST" ] \
  || ! grep -Fq 'name: clubfoundry-fresh-test' "$INSTALLER_UNDER_TEST" \
  || ! grep -Fq 'CONTAINER_NAME="clubfoundry-fresh-test"' "$INSTALLER_UNDER_TEST" \
  || ! grep -Fq 'UPDATER_NAME="clubfoundry-updater-fresh-test"' "$INSTALLER_UNDER_TEST" \
  || ! grep -Fq 'UPDATER_PORT=3005' "$INSTALLER_UNDER_TEST"; then
  echo "[ERROR] installer fixture is not fully isolated from the primary installation"
  exit 1
fi

pass=0
fail=0
assert() {
  if [ "$1" = "1" ]; then
    pass=$((pass + 1))
    echo "  [OK] $2"
  else
    fail=$((fail + 1))
    echo "  [FAIL] $2 ($3)"
  fi
}

echo "[SETUP] Verify no leftover fixtures"
docker rm -f "$CONT" "$UPD" 2>/dev/null || true
rm -rf "$APP_DIR" "$DATA_DIR"
[ ! -d "$APP_DIR" ] && [ ! -d "$DATA_DIR" ] && echo "  target dirs absent - OK"

echo ""
echo "[RUN] Installer with --mode=b on clean target"
# The installer is fully non-interactive — it prompts for nothing. API key +
# host are pre-seeded via env so install.sh verifies the TrueNAS connection
# inline (and the app skips its first-run setup gate). Port is auto-selected.
export CLM_TRUENAS_API_KEY="${CLM_TRUENAS_API_KEY:-}"
export CLM_TRUENAS_HOST="${CLM_TRUENAS_HOST:-http://localhost}"
if [ -z "$CLM_TRUENAS_API_KEY" ]; then
  echo "  [ERROR] CLM_TRUENAS_API_KEY must be set for the verify-TrueNAS step"
  exit 1
fi
bash "$INSTALLER_UNDER_TEST" \
  --mode=b \
  --app-dir="$APP_DIR" \
  --port="$PORT" 2>&1 | tail -20
echo ""

echo "[VERIFY]"

running_main=$(docker inspect "$CONT" --format "{{.State.Running}}" 2>/dev/null || echo "false")
assert "$([ "$running_main" = "true" ] && echo 1 || echo 0)" "main container running" "got $running_main"

running_upd=$(docker inspect "$UPD" --format "{{.State.Running}}" 2>/dev/null || echo "false")
assert "$([ "$running_upd" = "true" ] && echo 1 || echo 0)" "updater container running" "got $running_upd"

img_main=$(docker inspect "$CONT" --format "{{.Config.Image}}" 2>/dev/null)
assert "$([ "$img_main" = "$EXPECTED_MAIN_IMAGE" ] && echo 1 || echo 0)" "main image matches test fixture" "got $img_main"

[ -f "$APP_DIR/docker-compose.yml" ] && assert 1 "docker-compose.yml written" ""
[ ! -f "$APP_DIR/docker-compose.yml" ] && assert 0 "docker-compose.yml written" "missing"
[ -f "$DATA_DIR/.env" ] && assert 1 ".env written to DATA_DIR" ""
[ ! -f "$DATA_DIR/.env" ] && assert 0 ".env written to DATA_DIR" "missing"

# Give the app a moment to bind — container "Running" is not enough,
# JWT secret must be generated and the Fastify server must be up.
sleep 8
http=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "http://localhost:$PORT/health" || echo 000)
assert "$([ "$http" = "200" ] && echo 1 || echo 0)" "main /health responds 200 on port $PORT" "got $http"

sh_http=$(curl -s -o /dev/null -w "%{http_code}" -m 5 "http://localhost:$UPD_PORT/health" || echo 000)
assert "$([ "$sh_http" = "200" ] && echo 1 || echo 0)" "sidecar /health responds 200 on port $UPD_PORT" "got $sh_http"

env_port=$(grep -E "^CLM_PORT=" "$DATA_DIR/.env" 2>/dev/null | cut -d= -f2)
assert "$([ "$env_port" = "$PORT" ] && echo 1 || echo 0)" ".env has CLM_PORT=$PORT" "got $env_port"

version=$(curl -s -m 5 "http://localhost:$PORT/health" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('version','?'))" 2>/dev/null || echo "?")
if [ -n "$version" ] && [ "$version" != "?" ]; then
  assert 1 "/health reports version ($version)" ""
else
  assert 0 "/health reports version" "got $version"
fi

echo ""
echo "[TEARDOWN]"
docker stop "$CONT" "$UPD" 2>&1 | head -2
docker rm "$CONT" "$UPD" 2>&1 | head -2
rm -rf "$APP_DIR" "$DATA_DIR"
echo "  fixtures removed"

echo ""
echo "Result: $pass passed, $fail failed"
[ $fail -gt 0 ] && exit 1
echo "Fresh-install E2E green."
exit 0
