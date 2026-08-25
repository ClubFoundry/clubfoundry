#!/usr/bin/env bash
set -e
set -o pipefail

# Output and transfer helpers.
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
log() { echo -e "${CYAN}[ClubFoundry]${NC} $1"; }
ok() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
error() {
  echo -e "${RED}[✗]${NC} $1"
  exit 1
}
usage() {
  cat <<'EOF'
Usage: bash install.sh [options]

Options:
  --mode=a|b                  Install without or with the updater sidecar.
  --channel=stable|beta|lts   Select the release channel.
  --offline=DIR               Install from an air-gapped bundle.
  --app-dir=DIR               Store configuration and data under DIR.
  --port=PORT                 Set the web UI port.
  --usage-mode=MODE           Set commercial or noncommercial use for this instance.
  --link-token=TOKEN          Link the installation to an account.
  --update                    Refresh an existing installation.
  --clean-install             Delete existing ClubFoundry data and reinstall.
  --dry-run                   Print the resolved plan without host changes.
  -h, --help                  Show this help.
EOF
}
port_free() {
  local p="$1"
  ! ss -Hltn 2>/dev/null | awk -v suffix=":${p}" '
    substr($4, length($4) - length(suffix) + 1) == suffix { found = 1 }
    END { exit found ? 0 : 1 }
  '
}
fmt_elapsed() {
  local ms="$1"
  local s=$(((ms + 999) / 1000))
  [ "$s" -lt 1 ] && s=1
  local m=$((s / 60))
  s=$((s % 60))
  if [ "$m" -gt 0 ]; then echo "${m}m ${s}s"; else echo "${s}s"; fi
}
fetch_with_heartbeat() {
  local url="$1"
  local out="$2"
  local label="$3"
  local total
  total=$(curl -fsSL -m 5 -I "$url" 2>/dev/null | awk -F': *' 'tolower($1)=="content-length"{gsub(/\r/,"",$2);print $2}' | tail -1 || true)
  total="${total:-0}"
  local start_ms end_ms elapsed_ms rc
  start_ms=$(date +%s%3N 2>/dev/null || echo 0)
  if command -v pv >/dev/null 2>&1 && [ "$total" -gt 0 ]; then
    # -b bytes -p progress/% -r rate -N name. NO -t/-e: drops H:MM:SS timer
    # and ETA — both useless when the transfer is <1 second on a fast LAN.
    curl -fsSL -m 300 "$url" | pv -f -i 0.2 -s "$total" -b -p -r -N "$label" >"$out"
    rc=$?
  else
    curl -fsSL -m 300 -o "$out" "$url"
    rc=$?
  fi
  end_ms=$(date +%s%3N 2>/dev/null || echo 0)
  elapsed_ms=$((end_ms - start_ms))
  local sz_mb
  sz_mb=$(($(stat -c%s "$out" 2>/dev/null || echo 0) / 1024 / 1024))
  echo "    ${label}: completed in $(fmt_elapsed "$elapsed_ms") (${sz_mb} MB transferred)" >&2
  return $rc
}
pick_fastest_mirror() {
  local urls_json="$1"
  local legacy="$2"
  if [ -n "$urls_json" ] && [ "$urls_json" != "null" ] && [ "$urls_json" != "[]" ]; then
    local url
    while IFS= read -r url; do
      [ -z "$url" ] && continue
      if curl -fsSL --max-time 5 -I -o /dev/null "$url" 2>/dev/null; then
        echo "$url"
        return 0
      fi
    done < <(echo "$urls_json" | jq -r '.[]?')
  fi
  echo "$legacy"
}
resolve_persistent_app_dir() {
  command -v findmnt >/dev/null 2>&1 || return 1
  local mounts apps_source apps_pool pool_root candidates
  mounts=$(findmnt -rn -t zfs -o SOURCE,TARGET 2>/dev/null || true)
  [ -n "$mounts" ] || return 1

  # Prefer the data pool already selected for TrueNAS Apps.
  apps_source=$(printf '%s\n' "$mounts" | awk '$2 == "/mnt/.ix-apps" { print $1; exit }')
  if [ -n "$apps_source" ]; then
    apps_pool="${apps_source%%/*}"
    pool_root=$(printf '%s\n' "$mounts" | awk -v pool="$apps_pool" '$1 == pool { print $2; exit }')
    if [ -n "$pool_root" ] && [ -d "$pool_root" ] && [ -w "$pool_root" ]; then
      printf '%s/clubfoundry\n' "${pool_root%/}"
      return 0
    fi
  fi

  # Without an Apps pool, auto-select only when there is exactly one mounted
  # data-pool root. Multiple pools require an explicit operator choice.
  candidates=$(printf '%s\n' "$mounts" | awk '
    index($1, "/") == 0 && $1 != "boot-pool" && $2 ~ /^\/mnt\/[^/]+$/ { print $2 }
  ')
  if [ "$(printf '%s\n' "$candidates" | sed '/^$/d' | wc -l)" -eq 1 ]; then
    pool_root=$(printf '%s\n' "$candidates" | sed '/^$/d')
    if [ -d "$pool_root" ] && [ -w "$pool_root" ]; then
      printf '%s/clubfoundry\n' "${pool_root%/}"
      return 0
    fi
  fi
  return 1
}

# Command-line defaults and argument validation.
CONTAINER_NAME="clubfoundry"
UPDATER_NAME="clubfoundry-updater"
IMAGE="ghcr.io/clubfoundry/clubfoundry:latest"
UPDATER_IMAGE="ghcr.io/clubfoundry/updater:latest"
APP_DIR=""
DATA_DIR=""
PORT=3000
PORT_FALLBACKS="3080 3400 8723 8910 9444"
CLI_PORT=""       # set by --port=
CLI_USAGE_MODE="" # set by --usage-mode=commercial|noncommercial
CLI_LINK_TOKEN="" # set by --link-token= (Account Portal one-liner)
DRY_RUN=0         # set by --dry-run: resolve and print without host changes
UPDATER_PORT=3001
CLOUD_URL="${CLOUD_URL:-https://clubfoundry-cloud.mamont718.workers.dev}"
CHANNEL="stable" # --channel=beta / --channel=lts opt into other tracks
MODE=""
OFFLINE_DIR=""
UPDATE_MODE=0
CLEAN_INSTALL=0

for arg in "$@"; do
  case "$arg" in
    --mode=a | --mode=A) MODE="a" ;;
    --mode=b | --mode=B) MODE="b" ;;
    --channel=*) CHANNEL="${arg#--channel=}" ;;
    --offline=*) OFFLINE_DIR="${arg#--offline=}" ;;
    --app-dir=*) APP_DIR="${arg#--app-dir=}" ;;
    --port=*) CLI_PORT="${arg#--port=}" ;;
    --usage-mode=*) CLI_USAGE_MODE="${arg#--usage-mode=}" ;;
    --link-token=*) CLI_LINK_TOKEN="${arg#--link-token=}" ;;
    --update) UPDATE_MODE=1 ;;
    --clean-install) CLEAN_INSTALL=1 ;;
    --dry-run) DRY_RUN=1 ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) error "Unknown option: $arg. Run with --help for usage." ;;
  esac
done

if [ -n "$APP_DIR" ]; then
  case "$APP_DIR" in
    /*) ;;
    *) error "--app-dir must be an absolute path." ;;
  esac
  while [ "${APP_DIR%/}" != "$APP_DIR" ]; do
    APP_DIR="${APP_DIR%/}"
  done
  [ -n "$APP_DIR" ] || error "--app-dir cannot be the filesystem root."
fi
if [ -n "$CLI_PORT" ]; then
  [[ "$CLI_PORT" =~ ^[0-9]+$ ]] || error "--port must be an integer between 1 and 65535."
  if [ "$CLI_PORT" -lt 1 ] || [ "$CLI_PORT" -gt 65535 ]; then
    error "--port must be an integer between 1 and 65535."
  fi
fi
case "$CLI_USAGE_MODE" in
  ""|commercial|noncommercial) ;;
  *) error "Invalid --usage-mode: $CLI_USAGE_MODE (must be commercial or noncommercial)." ;;
esac

log "ClubFoundry Installer"
echo ""

# Host preflight and deployment-mode resolution.
if [ "$EUID" -ne 0 ]; then
  error "Please run as root: sudo bash install.sh"
fi
TN_VERSION=""
TN_MAJOR=""
TN_MINOR=""
if [ -r /etc/version ]; then
  TN_VERSION="$(tr -d '[:space:]' </etc/version)"
  TN_MAJOR="$(echo "$TN_VERSION" | cut -d. -f1)"
  TN_MINOR="$(echo "$TN_VERSION" | cut -d. -f2)"
fi
if ! command -v docker &>/dev/null; then
  if [ "$TN_MAJOR" = "24" ] && [ "$TN_MINOR" = "04" ]; then
    echo -e "${RED}[✗]${NC} TrueNAS SCALE Dragonfish ${TN_VERSION} detected — this version uses k3s, not native Docker."
    echo ""
    echo "    ClubFoundry needs native Docker, which arrived in 24.10 Electric Eel."
    echo "    Upgrade path:"
    echo "      1. System Settings → Update → Train → switch to TrueNAS-SCALE-ElectricEel"
    echo "      2. Download Updates → Apply (one reboot, ~10-20 min)"
    echo "      3. After boot: System Settings → Services → Docker → enable + start"
    echo "      4. Re-run this installer."
    echo ""
    echo "    Tip: bring 24.04 to the latest 24.04.x patch first before switching trains."
    echo "    Rollback: previous boot environment is retained, downgrade via System → Boot."
    echo ""
    error "Aborting. Re-run after upgrading to TrueNAS SCALE 24.10+ with Docker enabled."
  fi
  if [ "$TN_MAJOR" = "22" ] || [ "$TN_MAJOR" = "23" ]; then
    echo -e "${RED}[✗]${NC} TrueNAS SCALE ${TN_VERSION} detected — this version predates native Docker."
    echo ""
    echo "    ClubFoundry requires TrueNAS SCALE 24.10 Electric Eel or newer."
    echo "    Upgrade incrementally via System Settings → Update (do not skip trains)."
    echo ""
    error "Aborting. Upgrade to TrueNAS SCALE 24.10+ first."
  fi
  echo -e "${RED}[✗]${NC} Docker not found on \$PATH."
  echo ""
  if [ -n "$TN_VERSION" ]; then
    echo "    TrueNAS SCALE ${TN_VERSION} ships with Docker bundled, but the service is OFF by default."
  else
    echo "    TrueNAS SCALE 24.10+ ships with Docker bundled, but the service is OFF by default."
  fi
  echo "    Enable it:"
  echo "      1. Open the TrueNAS web UI"
  echo "      2. System Settings → Services → find the Docker row"
  echo "      3. Toggle Running, then Start Automatically"
  echo "      4. Wait ~10 seconds for the daemon to come up"
  echo "      5. SSH back in and re-run this installer."
  echo ""
  error "Aborting. Enable Docker in System Settings → Services, then re-run."
fi
if ! docker info &>/dev/null; then
  echo -e "${RED}[✗]${NC} Docker is installed but the daemon is not running."
  echo ""
  if [ "$TN_MAJOR" = "25" ] && [ "$TN_MINOR" = "10" ]; then
    echo "    TrueNAS SCALE ${TN_VERSION} (Goldeye) bundles Docker with the Apps service."
    echo "    Docker is NOT under System → Services in this version — start it via Apps:"
    echo "      1. Open the TrueNAS web UI"
    echo "      2. Left sidebar → Apps"
    echo "      3. A modal appears: \"Apps Service Not Configured / Choose a pool\""
    echo "      4. Pick any existing ZFS pool from the dropdown → Save"
    echo "      5. Wait ~30 seconds for the Apps service to start (Docker comes with it)"
    echo "      6. SSH back in and re-run this installer."
  elif [ "$TN_MAJOR" = "25" ] && [ "$TN_MINOR" = "04" ]; then
    echo "    TrueNAS SCALE ${TN_VERSION} (Fangtooth) has Docker in System → Services."
    echo "    Enable it:"
    echo "      1. Open the TrueNAS web UI"
    echo "      2. System Settings → Services → find the Docker row"
    echo "      3. Toggle Running, then Start Automatically"
    echo "      4. Wait ~10 seconds for the daemon to come up"
    echo "      5. SSH back in and re-run this installer."
  elif [ "$TN_MAJOR" = "24" ] && [ "$TN_MINOR" = "10" ]; then
    echo "    TrueNAS SCALE ${TN_VERSION} (Electric Eel) has Docker in System → Services."
    echo "    Enable it:"
    echo "      1. Open the TrueNAS web UI"
    echo "      2. System Settings → Services → find the Docker row"
    echo "      3. Toggle Running, then Start Automatically"
    echo "      4. Wait ~10 seconds for the daemon to come up"
    echo "      5. SSH back in and re-run this installer."
  else
    echo "    The Docker daemon is not responding on /var/run/docker.sock."
    if [ -n "$TN_VERSION" ]; then
      echo "    Detected TrueNAS SCALE ${TN_VERSION} — Docker location may vary by version."
    fi
    echo "    Try one of:"
    echo "      • System → Services → Docker (Electric Eel 24.10, Fangtooth 25.04)"
    echo "      • Apps → choose pool (Goldeye 25.10+)"
    echo "    Once the daemon is up, SSH back in and re-run this installer."
  fi
  echo ""
  error "Aborting. Start the Docker daemon, then re-run."
fi
HAS_COMPOSE=0
COMPOSE_CMD=""
if docker compose version &>/dev/null; then
  HAS_COMPOSE=1
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &>/dev/null; then
  HAS_COMPOSE=1
  COMPOSE_CMD="docker-compose"
fi
run_compose() {
  if [ "$COMPOSE_CMD" = "docker compose" ]; then
    docker compose "$@"
  else
    docker-compose "$@"
  fi
}
if [ -z "$APP_DIR" ]; then
  APP_DIR=$(resolve_persistent_app_dir) || error "Cannot select a persistent TrueNAS data pool automatically. Re-run with --app-dir=/mnt/<pool>/clubfoundry"
  log "Using persistent TrueNAS data-pool path $APP_DIR"
fi
DATA_DIR="${APP_DIR}/data"
if [ "$DRY_RUN" -eq 0 ]; then
  mkdir -p "$APP_DIR" 2>/dev/null || error "Cannot create $APP_DIR. Pass --app-dir=/path/to/writable/dir"
  [ -w "$APP_DIR" ] || error "$APP_DIR is not writable. Pass --app-dir=/path/to/writable/dir"
fi
case "$CHANNEL" in
  stable | beta | lts) ;;
  *) error "Invalid channel: $CHANNEL (must be stable, beta or lts)" ;;
esac
if [ -z "$MODE" ]; then
  if [ "$HAS_COMPOSE" -eq 1 ]; then
    MODE="b"
  else
    MODE="a"
    warn "docker compose not found — installing single-container Mode A (no auto-update sidecar)."
    warn "Install docker compose and re-run for auto-updates + safe rollback."
  fi
fi
MODE=$(echo "$MODE" | tr '[:upper:]' '[:lower:]')
if [ "$MODE" = "b" ] && [ "$HAS_COMPOSE" -eq 0 ]; then
  error "Mode B requires docker compose. Install it or re-run with --mode=a."
fi
if [ "$MODE" != "a" ] && [ "$MODE" != "b" ]; then
  error "Invalid mode: $MODE. Must be 'a' or 'b'."
fi
log "Selected mode: ${MODE^^}"

# Existing-installation ownership and cleanup policy.
# Ownership safety: refuse to touch a container with our name if it
# belongs to a non-ClubFoundry image (name collision with someone else's
# app). Image must match one of our known prefixes.
is_our_image() {
  case "$1" in
    clubfoundry:* | clubfoundry-updater:* | */clubfoundry:* | */clubfoundry-updater:*) return 0 ;;
    *) return 1 ;;
  esac
}
verify_ownership() {
  local c="$1"
  docker inspect "$c" >/dev/null 2>&1 || return 0
  local img
  img=$(docker inspect "$c" --format '{{.Config.Image}}' 2>/dev/null)
  is_our_image "$img" || error "Container '$c' exists but uses image '$img', which is not ClubFoundry. Refusing to manage it. Rename or 'docker rm -f $c' it yourself before re-running."
  if [ "$c" = "$CONTAINER_NAME" ]; then
    PREVIOUS_MAIN_IMAGE="$img"
  fi
}
PREVIOUS_MAIN_IMAGE=""
verify_ownership "$CONTAINER_NAME"
verify_ownership "$UPDATER_NAME"
HAS_CONTAINER=0
HAS_DATA=0
HAS_DB=0
DEFER_EXISTING_REFRESH=0
docker inspect "$CONTAINER_NAME" >/dev/null 2>&1 && HAS_CONTAINER=1
docker inspect "$UPDATER_NAME" >/dev/null 2>&1 && HAS_CONTAINER=1
[ -d "$DATA_DIR" ] && HAS_DATA=1
[ -f "$DATA_DIR/clm.db" ] && HAS_DB=1

# Per-instance licensing declaration. Existing installations keep an explicit
# value. A legacy install with no value writes `auto`; the updated backend then
# classifies it once from its managed non-admin PC count and persists the result.
USAGE_MODE=""
if [ "$HAS_DB" -eq 1 ] && [ "$CLEAN_INSTALL" -eq 0 ]; then
  USAGE_MODE=$(grep -E '^CLM_USAGE_MODE=' "${DATA_DIR}/.env" 2>/dev/null | head -1 | cut -d= -f2 | tr -d '[:space:]' || true)
  case "$USAGE_MODE" in
    commercial|noncommercial) ;;
    *) USAGE_MODE="auto" ;;
  esac
  case "$CLI_USAGE_MODE" in
    "") ;;
    commercial|noncommercial)
      if [ "$CLI_USAGE_MODE" != "$USAGE_MODE" ]; then
        warn "Changing instance usage mode from ${USAGE_MODE} to ${CLI_USAGE_MODE}."
        USAGE_MODE="$CLI_USAGE_MODE"
      fi
      ;;
  esac
  if [ "$USAGE_MODE" = "auto" ]; then
    ok "Instance usage mode: auto-select on first updated start"
  else
    ok "Instance usage mode: ${USAGE_MODE}"
  fi
else
  case "$CLI_USAGE_MODE" in
    commercial|noncommercial) USAGE_MODE="$CLI_USAGE_MODE" ;;
    "") ;;
  esac
  if [ -z "$USAGE_MODE" ] && [ -t 1 ] && [ -c /dev/tty ] && ( : </dev/tty ) 2>/dev/null; then
    echo ""
    echo "How will this ClubFoundry instance be used?"
    echo "  1) Personal / non-commercial use"
    echo "     Free released features, up to 30 managed PCs. No direct or indirect"
    echo "     business, employer, customer, computer-club, or internet-cafe use."
    echo "  2) Commercial / professional use"
    echo "     First 10 managed PCs are free; current pricing applies beyond that."
    while [ -z "$USAGE_MODE" ]; do
      read -r -p "Choose 1 or 2: " usage_choice </dev/tty
      case "$usage_choice" in
        1) USAGE_MODE="noncommercial" ;;
        2) USAGE_MODE="commercial" ;;
        *) warn "Enter 1 for non-commercial or 2 for commercial use." ;;
      esac
    done
  elif [ -z "$USAGE_MODE" ]; then
    USAGE_MODE="commercial"
    warn "Non-interactive install without --usage-mode; defaulting to commercial use."
  fi
  ok "Instance usage mode: ${USAGE_MODE}"
fi
stop_and_remove_ours() {
  # Try compose-down first — it knows the project name + drops orphans.
  if [ -f "${APP_DIR}/docker-compose.yml" ] && [ "$HAS_COMPOSE" -eq 1 ]; then
    (cd "${APP_DIR}" && run_compose down --remove-orphans 2>/dev/null) || true
  fi
  docker stop "$CONTAINER_NAME" "$UPDATER_NAME" 2>/dev/null || true
  docker rm -f "$CONTAINER_NAME" "$UPDATER_NAME" 2>/dev/null || true
}
if [ "$DRY_RUN" -eq 1 ]; then
  if [ "$CLEAN_INSTALL" -eq 1 ]; then
    warn "Dry run: existing ClubFoundry data would be deleted by --clean-install."
  elif [ "$HAS_DB" -eq 1 ]; then
    log "Dry run: existing database would be preserved and containers refreshed."
  elif [ "$HAS_CONTAINER" -eq 1 ] || [ "$HAS_DATA" -eq 1 ]; then
    log "Dry run: incomplete installation would be replaced."
  fi
elif [ "$CLEAN_INSTALL" -eq 1 ]; then
  warn "--clean-install: WIPING all prior state including database (destructive)..."
  stop_and_remove_ours
  rm -rf "$DATA_DIR"
elif [ "$HAS_DB" -eq 1 ]; then
  log "Existing ClubFoundry install with database detected — keeping it running while replacement images are prepared..."
  DEFER_EXISTING_REFRESH=1
elif [ "$HAS_CONTAINER" -eq 1 ] || [ "$HAS_DATA" -eq 1 ]; then
  log "Incomplete prior install detected (no database yet) — cleaning slate for fresh install..."
  stop_and_remove_ours
  rm -rf "$DATA_DIR"
fi
echo ""

# Installation configuration and dry-run boundary.
log "[1/10] Configuration"
echo ""
if [ -n "$CLM_TRUENAS_API_KEY" ] && [ -n "$CLM_TRUENAS_HOST" ]; then
  log "[2/10] Verifying pre-seeded TrueNAS connection..."
  HTTP_CODE=$(curl -s -m 10 -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer ${CLM_TRUENAS_API_KEY}" \
    "${CLM_TRUENAS_HOST}/api/v2.0/system/info" 2>/dev/null || echo "000")
  case "$HTTP_CODE" in
    200) ok "TrueNAS API connection verified" ;;
    401) error "Pre-seeded API key is invalid (401). Fix CLM_TRUENAS_API_KEY, or unset it to configure in the web UI." ;;
    000) warn "Cannot reach ${CLM_TRUENAS_HOST} right now — keeping creds; the web UI will re-verify." ;;
    *) warn "TrueNAS returned HTTP ${HTTP_CODE}; proceeding (the web UI will re-verify)." ;;
  esac
else
  CLM_TRUENAS_API_KEY=""
  CLM_TRUENAS_HOST=""
  log "[2/10] TrueNAS connection deferred to the in-app setup (browser copy-paste, 14 languages)."
fi
log "[3/10] Selecting host port..."
if [ -n "$CLI_PORT" ]; then
  PORT="$CLI_PORT"
  if ! port_free "$PORT"; then
    error "Requested --port=${PORT} is already in use on this host. Free it or pick another."
  fi
  ok "Using requested port ${PORT}"
elif { [ "$UPDATE_MODE" -eq 1 ] || [ "$DRY_RUN" -eq 1 ] || [ "$HAS_DB" -eq 1 ]; } && [ -f "${DATA_DIR}/.env" ]; then
  EXIST_PORT=$(grep -E '^CLM_PORT=' "${DATA_DIR}/.env" 2>/dev/null | head -1 | cut -d= -f2 | tr -d '[:space:]' || true)
  PORT="${EXIST_PORT:-$PORT}"
  ok "Reusing existing port ${PORT} (update — not moving a port clients already use)"
elif port_free "$PORT"; then
  ok "Port ${PORT} is free"
else
  CHOSEN=""
  for cand in $PORT_FALLBACKS; do
    if port_free "$cand"; then
      CHOSEN="$cand"
      break
    fi
  done
  [ -z "$CHOSEN" ] && error "Default port ${PORT} and all fallbacks (${PORT_FALLBACKS}) are occupied. Free one or pass --port=N."
  warn "Port ${PORT} is occupied — auto-selected free port ${CHOSEN}"
  PORT="$CHOSEN"
fi
if [ "$DRY_RUN" -eq 0 ] && [ "$MODE" = "b" ] && [ "$HAS_DB" -eq 0 ] && ! port_free "$UPDATER_PORT"; then
  error "Sidecar port ${UPDATER_PORT} is in use and cannot be moved (the app reaches the updater at a fixed localhost:${UPDATER_PORT}). Find the holder: ss -ltnp 'sport = :${UPDATER_PORT}' — free it and re-run, or install single-container with --mode=a (no auto-update)."
fi
log "[4/10] Processing account-link token..."
LINK_TOKEN=""
if [ -n "$CLI_LINK_TOKEN" ]; then
  if [[ "$CLI_LINK_TOKEN" =~ ^cflink_[A-Za-z0-9_-]{20,80}$ ]]; then
    LINK_TOKEN="$CLI_LINK_TOKEN"
    ok "Account-link token accepted — instance will bind to your account on first heartbeat"
  else
    warn "Ignoring malformed --link-token (expected cflink_…). Install continues; link later from the Account Portal."
  fi
fi
ENV_FILE="${DATA_DIR}/.env"
if [ "$DRY_RUN" -eq 1 ]; then
  echo ""
  log "DRY RUN - resolved plan (no files, images, or containers changed):"
  echo "  mode:          ${MODE}"
  echo "  channel:       ${CHANNEL}"
  echo "  web UI port:   ${PORT}"
  echo "  sidecar port:  ${UPDATER_PORT} (fixed)"
  echo "  app dir:       ${APP_DIR}"
  echo "  data dir:      ${DATA_DIR}"
  echo "  env file:      ${ENV_FILE}"
  echo "  usage mode:    ${USAGE_MODE}"
  echo "  link token:    $([ -n "$LINK_TOKEN" ] && echo "would be written" || echo "none")"
  echo "  truenas creds: $([ -n "$CLM_TRUENAS_API_KEY" ] && echo "pre-seeded" || echo "deferred to in-app gate")"
  ok "Dry run complete - re-run without --dry-run to install."
  exit 0
fi
# Image acquisition, integrity verification, and loading.
MAIN_IMAGE_READY=0
UPDATER_IMAGE_READY=0
if [ -n "$OFFLINE_DIR" ]; then
  [ -d "$OFFLINE_DIR" ] || error "Offline bundle directory not found: $OFFLINE_DIR"
  MANIFEST="${OFFLINE_DIR}/MANIFEST"
  SUMS="${OFFLINE_DIR}/SHA256SUMS"
  [ -f "$MANIFEST" ] || error "Bundle missing MANIFEST file at ${MANIFEST}"
  [ -f "$SUMS" ] || error "Bundle missing SHA256SUMS file at ${SUMS}"
  . "$MANIFEST"
  : "${MAIN_VERSION:?MANIFEST missing MAIN_VERSION}"
  : "${UPDATER_VERSION:?MANIFEST missing UPDATER_VERSION}"
  log "[6/10] Offline bundle: main=${MAIN_VERSION} updater=${UPDATER_VERSION}"
  log "Verifying bundle integrity (sha256)..."
  (cd "$OFFLINE_DIR" && sha256sum -c SHA256SUMS --quiet) \
    || error "Bundle integrity check FAILED — refusing to load tampered files."
  ok "Bundle integrity OK"
  TARBALL_MAIN="${OFFLINE_DIR}/images/clubfoundry-${MAIN_VERSION}.tar.gz"
  [ -f "$TARBALL_MAIN" ] || error "Main tarball not found in bundle: $TARBALL_MAIN"
  log "[8/10] Loading main image from $(basename "$TARBALL_MAIN") (Docker shows per-layer progress)..."
  TMP_LOG=$(mktemp)
  docker load -i "$TARBALL_MAIN" >"$TMP_LOG" || {
    rm -f "$TMP_LOG"
    error "docker load failed"
  }
  LOADED_IMAGE=$(grep -oE "Loaded image: [^ ]+" "$TMP_LOG" | awk '{print $3}' | head -1 || true)
  rm -f "$TMP_LOG"
  [ -z "$LOADED_IMAGE" ] && error "docker load produced no image tag"
  IMAGE="$LOADED_IMAGE"
  MAIN_IMAGE_READY=1
  ok "Loaded main image: $IMAGE"
  if [ "$MODE" = "b" ]; then
    TARBALL_UPD="${OFFLINE_DIR}/images/clubfoundry-updater-${UPDATER_VERSION}.tar.gz"
    [ -f "$TARBALL_UPD" ] || error "Updater tarball not found in bundle: $TARBALL_UPD"
    log "[10/10] Loading updater image from $(basename "$TARBALL_UPD") (Docker shows per-layer progress)..."
    TMP_LOG=$(mktemp)
    docker load -i "$TARBALL_UPD" >"$TMP_LOG" || {
      rm -f "$TMP_LOG"
      error "docker load failed (updater)"
    }
    LOADED_UPD=$(grep -oE "Loaded image: [^ ]+" "$TMP_LOG" | awk '{print $3}' | head -1 || true)
    rm -f "$TMP_LOG"
    [ -z "$LOADED_UPD" ] && error "docker load produced no updater tag"
    UPDATER_IMAGE="$LOADED_UPD"
    UPDATER_IMAGE_READY=1
    ok "Loaded updater image: $UPDATER_IMAGE"
  fi
  DOWNLOAD_URL="offline://${OFFLINE_DIR}"
else
  log "[6/10] Resolving latest ${CHANNEL} release from cloud..."
  CLOUD_RESP=$(curl -fsSL -m 15 "${CLOUD_URL}/api/update?current=0.0.0&channel=${CHANNEL}" 2>/dev/null || echo "")
  if [ -n "$CLOUD_RESP" ]; then
    LATEST_VER=$(echo "$CLOUD_RESP" | jq -r '.latest // empty')
    DOWNLOAD_URL=$(pick_fastest_mirror \
      "$(echo "$CLOUD_RESP" | jq -c '.downloadUrls // []')" \
      "$(echo "$CLOUD_RESP" | jq -r '.downloadUrl // empty')")
    DOWNLOAD_SHA=$(echo "$CLOUD_RESP" | jq -r '.downloadSha256 // empty')
    UPDATER_URL=$(pick_fastest_mirror \
      "$(echo "$CLOUD_RESP" | jq -c '.updaterDownloadUrls // []')" \
      "$(echo "$CLOUD_RESP" | jq -r '.updaterDownloadUrl // empty')")
    UPDATER_SHA=$(echo "$CLOUD_RESP" | jq -r '.updaterDownloadSha256 // empty')
    [ -n "$DOWNLOAD_URL" ] && log "Main mirror:    $DOWNLOAD_URL"
    [ -n "$UPDATER_URL" ] && log "Updater mirror: $UPDATER_URL"
  else
    warn "Could not reach ${CLOUD_URL} — falling back to registry pull"
    warn "For air-gapped installs, use: bash install.sh --offline=/path/to/bundle/"
  fi
  if [ -n "${DOWNLOAD_URL:-}" ] && [ -n "${DOWNLOAD_SHA:-}" ]; then
    TMP_MAIN=$(mktemp)
    log "[7/10] Fetching main image v${LATEST_VER} (~80 MB)..."
    fetch_with_heartbeat "$DOWNLOAD_URL" "$TMP_MAIN" "main image" \
      || {
        rm -f "$TMP_MAIN"
        error "Failed to fetch ${DOWNLOAD_URL}"
      }
    ACTUAL_SHA=$(sha256sum "$TMP_MAIN" | awk '{print $1}')
    if [ "$ACTUAL_SHA" != "$DOWNLOAD_SHA" ]; then
      rm -f "$TMP_MAIN"
      error "Main image sha256 mismatch (got $ACTUAL_SHA, expected $DOWNLOAD_SHA). Refusing to load unverified image."
    fi
    log "[8/10] Loading main image into Docker..."
    TMP_LOG=$(mktemp)
    LOAD_SIZE=$(stat -c%s "$TMP_MAIN" 2>/dev/null || echo 0)
    LOAD_START_MS=$(date +%s%3N 2>/dev/null || echo 0)
    if command -v pv >/dev/null 2>&1 && [ "$LOAD_SIZE" -gt 0 ]; then
      pv -f -i 0.2 -s "$LOAD_SIZE" -b -p -r -N "main image load" "$TMP_MAIN" | docker load >"$TMP_LOG" || {
        rm -f "$TMP_MAIN" "$TMP_LOG"
        error "docker load failed"
      }
    else
      docker load -i "$TMP_MAIN" >"$TMP_LOG" || {
        rm -f "$TMP_MAIN" "$TMP_LOG"
        error "docker load failed"
      }
    fi
    LOAD_END_MS=$(date +%s%3N 2>/dev/null || echo 0)
    echo "    main image load: completed in $(fmt_elapsed $((LOAD_END_MS - LOAD_START_MS))) ($((LOAD_SIZE / 1024 / 1024)) MB loaded)" >&2
    rm -f "$TMP_MAIN"
    LOADED_IMAGE=$(grep -oE "Loaded image: [^ ]+" "$TMP_LOG" | awk '{print $3}' | head -1 || true)
    rm -f "$TMP_LOG"
    [ -z "$LOADED_IMAGE" ] && error "docker load succeeded but produced no image tag"
    IMAGE="$LOADED_IMAGE"
    MAIN_IMAGE_READY=1
    ok "Loaded main image: $IMAGE"
  fi
  if [ "$MODE" = "b" ] && [ -n "${UPDATER_URL:-}" ] && [ -n "${UPDATER_SHA:-}" ]; then
    TMP_UPD=$(mktemp)
    log "[9/10] Fetching updater image (~37 MB)..."
    fetch_with_heartbeat "$UPDATER_URL" "$TMP_UPD" "updater" \
      || {
        rm -f "$TMP_UPD"
        error "Failed to fetch ${UPDATER_URL}"
      }
    ACTUAL_SHA=$(sha256sum "$TMP_UPD" | awk '{print $1}')
    if [ "$ACTUAL_SHA" != "$UPDATER_SHA" ]; then
      rm -f "$TMP_UPD"
      error "Updater image sha256 mismatch (got $ACTUAL_SHA, expected $UPDATER_SHA). Refusing to load unverified image."
    fi
    log "[10/10] Loading updater image into Docker..."
    TMP_LOG=$(mktemp)
    LOAD_SIZE=$(stat -c%s "$TMP_UPD" 2>/dev/null || echo 0)
    LOAD_START_MS=$(date +%s%3N 2>/dev/null || echo 0)
    if command -v pv >/dev/null 2>&1 && [ "$LOAD_SIZE" -gt 0 ]; then
      pv -f -i 0.2 -s "$LOAD_SIZE" -b -p -r -N "updater image load" "$TMP_UPD" | docker load >"$TMP_LOG" || {
        rm -f "$TMP_UPD" "$TMP_LOG"
        error "docker load failed (updater)"
      }
    else
      docker load -i "$TMP_UPD" >"$TMP_LOG" || {
        rm -f "$TMP_UPD" "$TMP_LOG"
        error "docker load failed (updater)"
      }
    fi
    LOAD_END_MS=$(date +%s%3N 2>/dev/null || echo 0)
    echo "    updater image load: completed in $(fmt_elapsed $((LOAD_END_MS - LOAD_START_MS))) ($((LOAD_SIZE / 1024 / 1024)) MB loaded)" >&2
    rm -f "$TMP_UPD"
    LOADED_UPD=$(grep -oE "Loaded image: [^ ]+" "$TMP_LOG" | awk '{print $3}' | head -1 || true)
    rm -f "$TMP_LOG"
    [ -z "$LOADED_UPD" ] && error "docker load succeeded but produced no updater tag"
    UPDATER_IMAGE="$LOADED_UPD"
    UPDATER_IMAGE_READY=1
    ok "Loaded updater image: $UPDATER_IMAGE"
  fi
fi

if [ "$MAIN_IMAGE_READY" -eq 0 ]; then
  log "Pulling ClubFoundry image..."
  docker pull "${IMAGE}"
  MAIN_IMAGE_READY=1
  ok "Main image pulled"
fi
if [ "$MODE" = "b" ] && [ "$UPDATER_IMAGE_READY" -eq 0 ]; then
  log "Pulling updater image..."
  docker pull "${UPDATER_IMAGE}"
  UPDATER_IMAGE_READY=1
  ok "Updater image pulled"
fi

ROLLBACK_DIR=""
PREVIOUS_ENV_PRESENT=0
PREVIOUS_LINK_TOKEN_PRESENT=0
PREVIOUS_COMPOSE_PRESENT=0
PREVIOUS_USAGE_MODE_REQUEST_PRESENT=0
cleanup_installer_rollback() {
  if [ -n "$ROLLBACK_DIR" ]; then
    rm -rf -- "$ROLLBACK_DIR"
    ROLLBACK_DIR=""
  fi
}
restore_previous_config() {
  if [ "$PREVIOUS_ENV_PRESENT" -eq 1 ]; then
    cp -a "${ROLLBACK_DIR}/.env" "$ENV_FILE"
  else
    rm -f "$ENV_FILE"
  fi
  if [ "$PREVIOUS_LINK_TOKEN_PRESENT" -eq 1 ]; then
    cp -a "${ROLLBACK_DIR}/.link-token" "${DATA_DIR}/.link-token"
  else
    rm -f "${DATA_DIR}/.link-token"
  fi
  if [ "$PREVIOUS_USAGE_MODE_REQUEST_PRESENT" -eq 1 ]; then
    cp -a "${ROLLBACK_DIR}/.usage-mode-request" "${DATA_DIR}/.usage-mode-request"
  else
    rm -f "${DATA_DIR}/.usage-mode-request"
  fi
  if [ "$PREVIOUS_COMPOSE_PRESENT" -eq 1 ]; then
    cp -a "${ROLLBACK_DIR}/docker-compose.yml" "${APP_DIR}/docker-compose.yml"
  else
    rm -f "${APP_DIR}/docker-compose.yml"
  fi
}
run_mode_a_container() {
  local image="$1"
  docker run -d \
    --name "${CONTAINER_NAME}" \
    --restart unless-stopped \
    --network host \
    --env-file "${ENV_FILE}" \
    -v "${DATA_DIR}:/app/data" \
    "$image"
}
prepare_installer_rollback() {
  ROLLBACK_DIR=$(mktemp -d /tmp/clubfoundry-installer-rollback.XXXXXX)
  chmod 700 "$ROLLBACK_DIR"
  if [ -f "$ENV_FILE" ]; then
    cp -a "$ENV_FILE" "${ROLLBACK_DIR}/.env"
    PREVIOUS_ENV_PRESENT=1
  fi
  if [ -f "${DATA_DIR}/.link-token" ]; then
    cp -a "${DATA_DIR}/.link-token" "${ROLLBACK_DIR}/.link-token"
    PREVIOUS_LINK_TOKEN_PRESENT=1
  fi
  if [ -f "${DATA_DIR}/.usage-mode-request" ]; then
    cp -a "${DATA_DIR}/.usage-mode-request" "${ROLLBACK_DIR}/.usage-mode-request"
    PREVIOUS_USAGE_MODE_REQUEST_PRESENT=1
  fi
  if [ -f "${APP_DIR}/docker-compose.yml" ]; then
    cp -a "${APP_DIR}/docker-compose.yml" "${ROLLBACK_DIR}/docker-compose.yml"
    PREVIOUS_COMPOSE_PRESENT=1
  fi
  trap cleanup_installer_rollback EXIT
}
restore_previous_mode_b() {
  restore_previous_config
  if [ "$PREVIOUS_COMPOSE_PRESENT" -eq 1 ]; then
    (cd "${APP_DIR}" && run_compose up -d)
    return
  fi
  [ -n "$PREVIOUS_MAIN_IMAGE" ] && run_mode_a_container "$PREVIOUS_MAIN_IMAGE"
}
if [ "$DEFER_EXISTING_REFRESH" -eq 1 ]; then
  prepare_installer_rollback
fi

# Commit configuration only after image acquisition succeeds. Existing
# containers remain usable if a download, checksum, or image load fails.
log "[5/10] Setting up data directory..."
mkdir -p "${DATA_DIR}"
cat >"${ENV_FILE}" <<EOF
CLM_TRUENAS_HOST=${CLM_TRUENAS_HOST}
CLM_TRUENAS_API_KEY=${CLM_TRUENAS_API_KEY}
CLM_PORT=${PORT}
CLM_DATA_DIR=/app/data
CLM_CLOUD_URL=${CLOUD_URL}
CLM_USAGE_MODE=${USAGE_MODE}
EOF
ok "Configuration saved to ${ENV_FILE}"
if [ "$HAS_DB" -eq 1 ] && [ "$CLEAN_INSTALL" -eq 0 ]; then
  case "$CLI_USAGE_MODE" in
    commercial|noncommercial)
      printf '%s\n' "$CLI_USAGE_MODE" >"${DATA_DIR}/.usage-mode-request"
      chmod 600 "${DATA_DIR}/.usage-mode-request" 2>/dev/null || true
      ;;
  esac
fi
if [ -n "$LINK_TOKEN" ]; then
  printf '%s\n' "$LINK_TOKEN" >"${DATA_DIR}/.link-token"
  ok "Account-link token written to ${DATA_DIR}/.link-token"
fi
chown -R 100:101 "${DATA_DIR}" 2>/dev/null || true
ok "Data directory: ${DATA_DIR}"

# Compose generation and container deployment.
write_mode_b_compose() {
  log "Writing docker-compose.yml..."
  mkdir -p "${APP_DIR}"
  COMPOSE_FILE="${APP_DIR}/docker-compose.yml"
  cat >"${COMPOSE_FILE}" <<YAML
# ClubFoundry — Mode B (with updater sidecar).
# Generated by installer/install.sh. Edit manually to customize; the
# installer overwrites this file on re-runs unless you rename it.

# Anchor the project name so compose invocations from the host and sidecar
# address the same services regardless of their working directories.
name: clubfoundry

services:
  clubfoundry:
    image: ${IMAGE}
    container_name: ${CONTAINER_NAME}
    network_mode: host
    restart: unless-stopped
    # Compose resolves this relative path from the compose file directory.
    # It works from both the host and the sidecar's mounted /app directory.
    env_file: ./data/.env
    volumes:
      - ${DATA_DIR}:/app/data

  clubfoundry-updater:
    image: ${UPDATER_IMAGE}
    container_name: ${UPDATER_NAME}
    # Host networking lets the sidecar probe the main app through the shared
    # loopback namespace. Bridge networking would probe the sidecar itself.
    network_mode: host
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      # Read-write is required because stepped updates change the main
      # service image between hops. A read-only mount breaks that contract.
      - ${COMPOSE_FILE}:/app/docker-compose.yml
      - ${DATA_DIR}:/app/data
    environment:
      CLUBFOUNDRY_CLOUD_URL: ${CLOUD_URL}
      CLUBFOUNDRY_COMPOSE_DIR: /app
      # Bind sidecar API to loopback inside the host network namespace.
      # Main reaches it via 127.0.0.1:3001 but it's not externally exposed.
      CLUBFOUNDRY_UPDATER_ADDR: 127.0.0.1:${UPDATER_PORT}
      # Pin the probe to the selected main-app port. The sidecar also reads
      # CLM_PORT from /app/data/.env when compose is regenerated.
      CLUBFOUNDRY_HEALTH_URL: http://127.0.0.1:${PORT}/health
YAML
  ok "Wrote ${COMPOSE_FILE}"
}
finish_installer_rollback() {
  cleanup_installer_rollback
  trap - EXIT
}
deploy_mode_b() {
  write_mode_b_compose
  ok "Images are ready locally"
  if [ "$DEFER_EXISTING_REFRESH" -eq 1 ]; then
    log "Replacement images are ready — refreshing existing containers..."
    stop_and_remove_ours
  fi
  log "Starting ClubFoundry via docker compose..."
  if (cd "${APP_DIR}" && run_compose up -d); then
    finish_installer_rollback
    return
  fi
  if [ "$DEFER_EXISTING_REFRESH" -eq 1 ]; then
    warn "Replacement stack failed to start — restoring the previous installation..."
    stop_and_remove_ours
    if restore_previous_mode_b; then
      finish_installer_rollback
      error "Replacement stack failed to start. Previous installation was restored."
    fi
    error "Replacement stack failed to start, and the previous installation could not be restarted automatically. Database and previous configuration were preserved in ${DATA_DIR}."
  fi
  error "ClubFoundry stack failed to start."
}
deploy_mode_a() {
  ok "Main image is ready locally"
  if [ "$DEFER_EXISTING_REFRESH" -eq 1 ]; then
    log "Replacement image is ready — refreshing existing container..."
    stop_and_remove_ours
  fi
  log "Starting ClubFoundry..."
  if run_mode_a_container "$IMAGE"; then
    finish_installer_rollback
  elif [ "$DEFER_EXISTING_REFRESH" -eq 1 ] && [ -n "$PREVIOUS_MAIN_IMAGE" ]; then
    warn "Replacement container failed to start — restoring previous Mode A installation..."
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
    restore_previous_config
    if run_mode_a_container "$PREVIOUS_MAIN_IMAGE"; then
      finish_installer_rollback
      error "Replacement container failed to start. Previous installation was restored with image ${PREVIOUS_MAIN_IMAGE}."
    fi
    error "Replacement container failed to start, and the previous image could not be restored automatically. Database and previous configuration were preserved in ${DATA_DIR}."
  else
    error "ClubFoundry container failed to start."
  fi
}
if [ "$MODE" = "b" ]; then
  deploy_mode_b
else
  deploy_mode_a
fi

wait_for_health() {
  local url="$1"
  local max_seconds="$2"
  local progress_every="$3"
  local label="$4"
  local i
  for i in $(seq 1 "$max_seconds"); do
    if curl -s "$url" >/dev/null 2>&1; then
      return 0
    fi
    if [ $((i % progress_every)) -eq 0 ]; then
      echo "    ${label}: ${i}s elapsed (max ${max_seconds}s)..." >&2
    fi
    sleep 1
  done
  return 1
}

# Health timeout is advisory. A successful Docker/Compose launch is not rolled
# back merely because a slow first boot needs more than the observation window.
log "Waiting for ClubFoundry to start (up to 30s)..."
if wait_for_health "http://localhost:${PORT}/health" 30 5 "health-check"; then
  ok "ClubFoundry is running!"
else
  warn "ClubFoundry may still be starting. Check: docker logs ${CONTAINER_NAME}"
fi
if [ "$MODE" = "b" ]; then
  log "Waiting for updater sidecar (up to 10s)..."
  if wait_for_health "http://localhost:${UPDATER_PORT}/health" 10 3 "sidecar health-check"; then
    ok "Updater sidecar is running!"
  else
    warn "Updater sidecar not responding. Check: docker logs ${UPDATER_NAME}"
  fi
fi
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo -e "  ${GREEN}ClubFoundry installed successfully!${NC} (Mode ${MODE^^})"
echo ""
WEB_UI_URL="http://$(hostname -I | awk '{print $1}' || true):${PORT}"
echo -e "  Web UI:    ${CYAN}${WEB_UI_URL}${NC}"
echo -e "  Data:      ${DATA_DIR}"
echo -e "  Container: ${CONTAINER_NAME}"
if [ "$MODE" = "b" ]; then
  echo -e "  Updater:   ${UPDATER_NAME} (port ${UPDATER_PORT}, auto-updates default OFF)"
  echo -e "  Compose:   ${APP_DIR}/docker-compose.yml"
fi
echo ""
echo "  Next steps:"
echo -e "  1. Open in your browser: ${GREEN}${WEB_UI_URL}${NC}"
echo "  2. Create an admin account"
if [ -z "$CLM_TRUENAS_API_KEY" ]; then
  echo "  3. Connect to TrueNAS in the browser form that appears:"
  echo "     • API key: TrueNAS Web UI → top-right user menu → API Keys → Add"
  echo "     • Host URL: default http://localhost is correct when ClubFoundry"
  echo "       runs on this TrueNAS; use http://<other-ip> for a remote one"
  echo "  4. Run the Setup Wizard"
else
  echo "  3. Run the Setup Wizard (TrueNAS credentials were pre-seeded)"
fi
if [ -n "$LINK_TOKEN" ]; then
  echo ""
  echo "  This instance links to your ClubFoundry account automatically"
  echo "  within ~1 minute — no further action needed."
fi
if [ "$MODE" = "b" ]; then
  echo ""
  echo "  Auto-update + safe rollback are active (updater sidecar)."
  echo "  Tune the window any time in Settings → Updates."
fi
echo ""
echo "  For more info or contacts, see https://clubfoundry.net"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
