#!/usr/bin/env bash
set -euo pipefail

INSTALLER="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/install.sh"
TEST_ROOT="$(mktemp -d /tmp/clubfoundry-installer-contracts.XXXXXX)"
DOCKER_CALLS="$TEST_ROOT/docker.calls"
FIXTURE_IMAGE_PAYLOAD="fixture image payload"
FIXTURE_IMAGE_SHA=$(printf '%s' "$FIXTURE_IMAGE_PAYLOAD" | sha256sum | awk '{print $1}')
FIXTURE_UPDATER_PAYLOAD="fixture updater payload"
FIXTURE_UPDATER_SHA=$(printf '%s' "$FIXTURE_UPDATER_PAYLOAD" | sha256sum | awk '{print $1}')
trap 'rm -rf -- "$TEST_ROOT"' EXIT

docker() {
  local load_call
  printf '%s\n' "$*" >>"$DOCKER_CALLS"
  case "$*" in
    load | load\ -i\ *)
      load_call=$(grep -Ec '^load($| -i )' "$DOCKER_CALLS")
      if [ "${TEST_LOAD_FAILURE_ON:-}" = "$load_call" ]; then
        return 74
      fi
      if [ "${TEST_LOAD_NO_TAG_ON:-}" = "$load_call" ]; then
        printf '%s\n' 'Loaded image ID: sha256:fixture-without-tag'
        return 0
      fi
      ;;
  esac
  if [ "${TEST_REPLACEMENT_COMPOSE_FAILURE:-}" = "1" ]; then
    case "$*" in
      pull\ * | stop\ * | rm\ -f\ * | compose\ down\ *) return 0 ;;
      compose\ up\ -d)
        if [ "$(grep -Ec '^compose up -d$' "$DOCKER_CALLS")" -eq 1 ]; then
          return 17
        fi
        [ "${TEST_ROLLBACK_COMPOSE_FAILURE:-}" != "1" ]
        ;;
    esac
  fi
  if [ "${TEST_REPLACEMENT_RUN_FAILURE:-}" = "1" ]; then
    case "$*" in
      "pull ghcr.io/clubfoundry/clubfoundry:latest" | stop\ * | rm\ -f\ *) return 0 ;;
      run\ *ghcr.io/clubfoundry/clubfoundry:latest) return 125 ;;
      run\ *ghcr.io/clubfoundry/clubfoundry:1.2.3)
        [ "${TEST_ROLLBACK_RUN_FAILURE:-}" != "1" ]
        ;;
    esac
  fi
  if [ "${TEST_DEPLOY_SUCCESS:-}" = "1" ]; then
    case "$*" in
      pull\ * | run\ * | compose\ up\ -d) return 0 ;;
    esac
  fi
  if [ "${TEST_ALL_PULL_SUCCESS:-}" = "1" ]; then
    case "$*" in
      pull\ *) return 0 ;;
    esac
  fi
  if [ "${TEST_MAIN_PULL_SUCCESS:-}" = "1" ]; then
    case "$*" in
      "pull ghcr.io/clubfoundry/clubfoundry:latest") return 0 ;;
    esac
  fi
  if [ "${TEST_LOAD_SUCCESS:-}" = "1" ]; then
    case "$*" in
      load | load\ -i\ *)
        printf '%s\n' 'Loaded image: fixtures/main:9.9.9'
        return 0
        ;;
    esac
  fi
  if [ "${TEST_FOREIGN_CONTAINER:-}" = "1" ]; then
    case "$*" in
      "inspect clubfoundry") return 0 ;;
      "inspect clubfoundry --format {{.Config.Image}}")
        printf '%s\n' "postgres:16"
        return 0
        ;;
    esac
  fi
  if [ "${TEST_EXISTING_INSTALL:-}" = "1" ]; then
    case "$*" in
      "inspect clubfoundry") return 0 ;;
      "inspect clubfoundry --format {{.Config.Image}}")
        printf '%s\n' "ghcr.io/clubfoundry/clubfoundry:1.2.3"
        return 0
        ;;
    esac
  fi
  case "$*" in
    info)
      [ "${TEST_DOCKER_DAEMON_DOWN:-}" != "1" ]
      ;;
    "compose version")
      [ "${TEST_COMPOSE_MISSING:-}" != "1" ] \
        && [ "${TEST_COMPOSE_PLUGIN_MISSING:-}" != "1" ]
      ;;
    inspect\ *) return 1 ;;
    *) return 97 ;;
  esac
}

docker-compose() {
  printf 'legacy-compose %s\n' "$*" >>"$DOCKER_CALLS"
  if [ "${TEST_DEPLOY_SUCCESS:-}" = "1" ]; then
    case "$*" in
      up\ -d | down\ --remove-orphans) return 0 ;;
    esac
  fi
  return 97
}

command() {
  if [ "$1" = "-v" ]; then
    if [ "$2" = "docker" ] && [ "${TEST_DOCKER_COMMAND_MISSING:-}" = "1" ]; then
      return 1
    fi
    if [ "$2" = "docker-compose" ] && [ "${TEST_COMPOSE_MISSING:-}" = "1" ]; then
      return 1
    fi
  fi
  builtin command "$@"
}

curl() {
  local output_path=""
  local previous=""
  local argument
  for argument in "$@"; do
    if [ "$previous" = "-o" ]; then
      output_path="$argument"
      break
    fi
    previous="$argument"
  done
  if [ "${TEST_HEALTH_DOWN:-}" = "1" ]; then
    case "$*" in
      *"/api/update?current=0.0.0"* | *"localhost:"*"/health"*) return 22 ;;
    esac
  fi
  if [ -n "${TEST_TRUENAS_HTTP_CODE:-}" ] && [[ "$*" == *"/api/v2.0/system/info"* ]]; then
    printf '%s' "$TEST_TRUENAS_HTTP_CODE"
    return 0
  fi
  if [ -n "${TEST_DOWNLOAD_MODE:-}" ]; then
    if [[ "$*" == *"/api/update?current=0.0.0"* ]]; then
      printf '%s' '{"latest":"9.9.9","downloadUrls":[],"downloadUrl":"https://fixtures.invalid/main.tar.gz","downloadSha256":"0000000000000000000000000000000000000000000000000000000000000000"}'
      return 0
    fi
    if [[ "$*" == *"https://fixtures.invalid/main.tar.gz"* ]]; then
      [[ " $* " == *" -I "* ]] && return 0
      case "$TEST_DOWNLOAD_MODE" in
        hash-mismatch | updater-interrupted | valid-both)
          if [ -n "$output_path" ]; then
            printf '%s' "$FIXTURE_IMAGE_PAYLOAD" >"$output_path"
          else
            printf '%s' "$FIXTURE_IMAGE_PAYLOAD"
          fi
          return 0
          ;;
        mirror-failure) return 22 ;;
        interrupted)
          if [ -n "$output_path" ]; then
            printf '%s' 'partial fixture' >"$output_path"
          else
            printf '%s' 'partial fixture'
          fi
          return 18
          ;;
      esac
    fi
    if [[ "$*" == *"https://fixtures.invalid/updater.tar.gz"* ]]; then
      [[ " $* " == *" -I "* ]] && return 0
      if [ "${TEST_DOWNLOAD_MODE:-}" = "valid-both" ]; then
        if [ -n "$output_path" ]; then
          printf '%s' "$FIXTURE_UPDATER_PAYLOAD" >"$output_path"
        else
          printf '%s' "$FIXTURE_UPDATER_PAYLOAD"
        fi
        return 0
      fi
      if [ -n "$output_path" ]; then
        printf '%s' 'partial updater fixture' >"$output_path"
      else
        printf '%s' 'partial updater fixture'
      fi
      return 18
    fi
  fi
  echo "unexpected curl call: $*" >&2
  return 96
}

jq() {
  cat >/dev/null
  case "$*" in
    *'.latest // empty'*) printf '%s\n' '9.9.9' ;;
    *'.downloadUrls // []'*) printf '%s\n' '[]' ;;
    *'.downloadUrl // empty'*) printf '%s\n' 'https://fixtures.invalid/main.tar.gz' ;;
    *'.downloadSha256 // empty'*)
      if [ "${TEST_UPDATER_DOWNLOAD:-}" = "1" ] || [ "${TEST_DOWNLOAD_MODE:-}" = "valid-both" ]; then
        printf '%s\n' "$FIXTURE_IMAGE_SHA"
      else
        printf '%064d\n' 0
      fi
      ;;
    *'.updaterDownloadUrls // []'*) printf '%s\n' '[]' ;;
    *'.updaterDownloadUrl // empty'*)
      { [ "${TEST_UPDATER_DOWNLOAD:-}" = "1" ] || [ "${TEST_DOWNLOAD_MODE:-}" = "valid-both" ]; } \
        && printf '%s\n' 'https://fixtures.invalid/updater.tar.gz'
      ;;
    *'.updaterDownloadSha256 // empty'*)
      if [ "${TEST_DOWNLOAD_MODE:-}" = "valid-both" ]; then
        printf '%s\n' "$FIXTURE_UPDATER_SHA"
      elif [ "${TEST_UPDATER_DOWNLOAD:-}" = "1" ]; then
        printf '%064d\n' 0
      fi
      ;;
    *'.[]?'*) ;;
    *)
      echo "unexpected jq call: $*" >&2
      return 95
      ;;
  esac
  return 0
}

sleep() {
  if [ "${TEST_FAST_SLEEP:-}" = "1" ]; then
    return 0
  fi
  /usr/bin/sleep "$@"
}

mktemp() {
  if [ -n "${TEST_INSTALLER_TMP_ROOT:-}" ]; then
    if [ "${1:-}" = "-d" ]; then
      /usr/bin/mktemp -d "$TEST_INSTALLER_TMP_ROOT/installer-tmp.XXXXXX"
    else
      /usr/bin/mktemp "$TEST_INSTALLER_TMP_ROOT/installer-tmp.XXXXXX"
    fi
  else
    /usr/bin/mktemp "$@"
  fi
}

ss() {
  if [ "${TEST_OCCUPIED_PORT:-}" = "41037" ]; then
    printf 'LISTEN 0 4096 0.0.0.0:41037 0.0.0.0:*\n'
  fi
}

export -f command curl docker docker-compose jq mktemp sleep ss
export DOCKER_CALLS FIXTURE_IMAGE_PAYLOAD FIXTURE_IMAGE_SHA FIXTURE_UPDATER_PAYLOAD FIXTURE_UPDATER_SHA

for shim_name in command curl docker docker-compose jq mktemp sleep ss; do
  if [ "$(bash -c "type -t $shim_name")" != "function" ]; then
    echo "$shim_name test shim was not inherited by a child Bash process" >&2
    exit 1
  fi
done

expect_rejected() {
  local description="$1"
  shift
  local output_file="$TEST_ROOT/rejected-output.log"
  if bash "$INSTALLER" "$@" >"$output_file" 2>&1; then
    echo "$description was accepted" >&2
    rm -f "$output_file"
    exit 1
  fi
  if [ "${DEBUG_INSTALLER_CONTRACTS:-}" = "1" ]; then
    echo "--- rejected: $description ---" >&2
    cat "$output_file" >&2
  fi
  rm -f "$output_file"
}

bash "$INSTALLER" --help >/dev/null
expect_rejected "unknown option" --unknown-option
expect_rejected "zero port" --port=0
expect_rejected "out-of-range port" --port=65536
expect_rejected "non-numeric port" --port=not-a-port
expect_rejected "relative app directory" --app-dir=relative/path
expect_rejected "filesystem-root app directory" --app-dir=/

missing_docker_target="$TEST_ROOT/missing-docker"
export TEST_DOCKER_COMMAND_MISSING=1
expect_rejected "missing Docker command" --dry-run --app-dir="$missing_docker_target"
unset TEST_DOCKER_COMMAND_MISSING
test ! -e "$missing_docker_target"

daemon_down_target="$TEST_ROOT/daemon-down"
export TEST_DOCKER_DAEMON_DOWN=1
expect_rejected "stopped Docker daemon" --dry-run --app-dir="$daemon_down_target"
unset TEST_DOCKER_DAEMON_DOWN
test ! -e "$daemon_down_target"

readonly_target="/proc/clubfoundry-installer-contracts"
expect_rejected "uncreatable application directory" \
  --mode=a \
  --port=41036 \
  --app-dir="$readonly_target"
test ! -e "$readonly_target"

compose_target="$TEST_ROOT/missing-compose"
export TEST_COMPOSE_MISSING=1
expect_rejected "Mode B without Compose" --dry-run --mode=b --app-dir="$compose_target"
compose_output=$(bash "$INSTALLER" --dry-run --app-dir="$compose_target")
unset TEST_COMPOSE_MISSING
printf '%s\n' "$compose_output" | grep -Fq "mode:          a"
test ! -e "$compose_target"

fresh_target="$TEST_ROOT/fresh"
bash "$INSTALLER" --dry-run --mode=b --port=41037 --app-dir="$fresh_target" >/dev/null
test ! -e "$fresh_target"

legacy_target="$TEST_ROOT/legacy"
mkdir -p "$legacy_target/data"
printf 'LEGACY_SETTING=preserve-me\n' >"$legacy_target/data/.env"
bash "$INSTALLER" --dry-run --mode=b --app-dir="$legacy_target" >/dev/null
test "$(cat "$legacy_target/data/.env")" = "LEGACY_SETTING=preserve-me"
test "$(find "$legacy_target" -type f | wc -l)" -eq 1

occupied_target="$TEST_ROOT/occupied"
if TEST_OCCUPIED_PORT=41037 bash "$INSTALLER" --dry-run --mode=b --port=41037 --app-dir="$occupied_target" >/dev/null 2>&1; then
  echo "occupied port was accepted" >&2
  exit 1
fi
test ! -e "$occupied_target"

secret_api="contract-api-secret-value"
secret_link="cflink_12345678901234567890"
export TEST_TRUENAS_HTTP_CODE=200
secret_output=$(CLM_TRUENAS_API_KEY="$secret_api" \
  CLM_TRUENAS_HOST="http://truenas.fixture" \
  bash "$INSTALLER" \
  --dry-run \
  --mode=b \
  --port=41039 \
  --app-dir="$TEST_ROOT/secret-output" \
  --link-token="$secret_link" 2>&1)
unset TEST_TRUENAS_HTTP_CODE
if [[ "$secret_output" == *"$secret_api"* ]] || [[ "$secret_output" == *"$secret_link"* ]]; then
  echo "dry-run output exposed a secret value" >&2
  exit 1
fi
test ! -e "$TEST_ROOT/secret-output"

foreign_target="$TEST_ROOT/foreign-container"
export TEST_FOREIGN_CONTAINER=1
expect_rejected "foreign same-named container" --dry-run --mode=b --app-dir="$foreign_target"
unset TEST_FOREIGN_CONTAINER
test ! -e "$foreign_target"

offline_target="$TEST_ROOT/missing-offline-bundle"
expect_rejected "missing offline bundle" \
  --mode=a \
  --port=41038 \
  --app-dir="$offline_target" \
  --offline="$TEST_ROOT/does-not-exist"

missing_manifest_bundle="$TEST_ROOT/offline-missing-manifest"
mkdir -p "$missing_manifest_bundle/images"
printf '%s\n' 'placeholder' >"$missing_manifest_bundle/SHA256SUMS"
expect_rejected "offline bundle without MANIFEST" \
  --mode=a \
  --port=41053 \
  --app-dir="$TEST_ROOT/offline-missing-manifest-target" \
  --offline="$missing_manifest_bundle"

missing_sums_bundle="$TEST_ROOT/offline-missing-sums"
mkdir -p "$missing_sums_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' 'UPDATER_VERSION=9.9.9' >"$missing_sums_bundle/MANIFEST"
expect_rejected "offline bundle without SHA256SUMS" \
  --mode=a \
  --port=41054 \
  --app-dir="$TEST_ROOT/offline-missing-sums-target" \
  --offline="$missing_sums_bundle"

missing_main_version_bundle="$TEST_ROOT/offline-missing-main-version"
mkdir -p "$missing_main_version_bundle/images"
printf '%s\n' 'UPDATER_VERSION=9.9.9' >"$missing_main_version_bundle/MANIFEST"
(cd "$missing_main_version_bundle" && sha256sum MANIFEST >SHA256SUMS)
expect_rejected "offline MANIFEST without MAIN_VERSION" \
  --mode=a \
  --port=41055 \
  --app-dir="$TEST_ROOT/offline-missing-main-version-target" \
  --offline="$missing_main_version_bundle"

missing_updater_version_bundle="$TEST_ROOT/offline-missing-updater-version"
mkdir -p "$missing_updater_version_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' >"$missing_updater_version_bundle/MANIFEST"
(cd "$missing_updater_version_bundle" && sha256sum MANIFEST >SHA256SUMS)
expect_rejected "offline MANIFEST without UPDATER_VERSION" \
  --mode=a \
  --port=41056 \
  --app-dir="$TEST_ROOT/offline-missing-updater-version-target" \
  --offline="$missing_updater_version_bundle"

missing_main_tarball_bundle="$TEST_ROOT/offline-missing-main-tarball"
mkdir -p "$missing_main_tarball_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' 'UPDATER_VERSION=9.9.9' >"$missing_main_tarball_bundle/MANIFEST"
(cd "$missing_main_tarball_bundle" && sha256sum MANIFEST >SHA256SUMS)
expect_rejected "offline bundle without main tarball" \
  --mode=a \
  --port=41057 \
  --app-dir="$TEST_ROOT/offline-missing-main-tarball-target" \
  --offline="$missing_main_tarball_bundle"

bad_bundle="$TEST_ROOT/bad-offline-bundle"
mkdir -p "$bad_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' 'UPDATER_VERSION=9.9.9' >"$bad_bundle/MANIFEST"
printf '%s' 'tampered image payload' >"$bad_bundle/images/clubfoundry-9.9.9.tar.gz"
printf '%064d  %s\n' 0 'images/clubfoundry-9.9.9.tar.gz' >"$bad_bundle/SHA256SUMS"
expect_rejected "offline checksum mismatch" \
  --mode=a \
  --port=41040 \
  --app-dir="$TEST_ROOT/bad-offline-target" \
  --offline="$bad_bundle"

export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
temp_shim_probe=$(bash -c 'mktemp')
unset TEST_INSTALLER_TMP_ROOT
case "$temp_shim_probe" in
  "$TEST_ROOT"/installer-tmp.*) ;;
  *)
    echo "installer temp-file shim was not inherited" >&2
    exit 1
    ;;
esac
rm -f "$temp_shim_probe"

export TEST_DOWNLOAD_MODE=hash-mismatch
curl_shim_probe=$(bash -c 'curl https://cloud.fixture/api/update?current=0.0.0')
unset TEST_DOWNLOAD_MODE
[[ "$curl_shim_probe" == *'"downloadUrl":"https://fixtures.invalid/main.tar.gz"'* ]] \
  || {
    echo "installer curl shim was not inherited" >&2
    exit 1
  }

for download_mode in hash-mismatch mirror-failure interrupted; do
  export TEST_DOWNLOAD_MODE="$download_mode"
  export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
  expect_rejected "$download_mode download" \
    --mode=a \
    --port=41041 \
    --app-dir="$TEST_ROOT/download-$download_mode"
  unset TEST_DOWNLOAD_MODE TEST_INSTALLER_TMP_ROOT
  if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
    echo "$download_mode left a temporary download behind" >&2
    exit 1
  fi
done

if grep -Eq '(^| )(stop|rm|pull|load|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "a pre-mutation safety scenario issued a lifecycle-changing Docker call" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
missing_updater_tarball_bundle="$TEST_ROOT/offline-missing-updater-tarball"
mkdir -p "$missing_updater_tarball_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' 'UPDATER_VERSION=9.9.9' >"$missing_updater_tarball_bundle/MANIFEST"
printf '%s' "$FIXTURE_IMAGE_PAYLOAD" >"$missing_updater_tarball_bundle/images/clubfoundry-9.9.9.tar.gz"
(cd "$missing_updater_tarball_bundle" \
  && sha256sum MANIFEST images/clubfoundry-9.9.9.tar.gz >SHA256SUMS)
export TEST_LOAD_SUCCESS=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "offline bundle without updater tarball" \
  --mode=b \
  --port=41058 \
  --app-dir="$TEST_ROOT/offline-missing-updater-tarball-target" \
  --offline="$missing_updater_tarball_bundle"
unset TEST_LOAD_SUCCESS TEST_INSTALLER_TMP_ROOT
test "$(grep -Ec '^load($| -i )' "$DOCKER_CALLS")" -eq 1
if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "missing updater tarball reached container deployment" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "missing updater tarball left a temporary log behind" >&2
  exit 1
fi

valid_offline_bundle="$TEST_ROOT/offline-valid-load-fixture"
mkdir -p "$valid_offline_bundle/images"
printf '%s\n' 'MAIN_VERSION=9.9.9' 'UPDATER_VERSION=9.9.9' >"$valid_offline_bundle/MANIFEST"
printf '%s' "$FIXTURE_IMAGE_PAYLOAD" >"$valid_offline_bundle/images/clubfoundry-9.9.9.tar.gz"
printf '%s' "$FIXTURE_UPDATER_PAYLOAD" >"$valid_offline_bundle/images/clubfoundry-updater-9.9.9.tar.gz"
(cd "$valid_offline_bundle" \
  && sha256sum MANIFEST images/clubfoundry-9.9.9.tar.gz \
    images/clubfoundry-updater-9.9.9.tar.gz >SHA256SUMS)

for load_case in main-failure main-no-tag updater-failure updater-no-tag; do
  : >"$DOCKER_CALLS"
  load_mode=a
  case "$load_case" in
    main-failure) export TEST_LOAD_FAILURE_ON=1 ;;
    main-no-tag) export TEST_LOAD_NO_TAG_ON=1 ;;
    updater-failure)
      load_mode=b
      export TEST_LOAD_FAILURE_ON=2
      ;;
    updater-no-tag)
      load_mode=b
      export TEST_LOAD_NO_TAG_ON=2
      ;;
  esac
  export TEST_LOAD_SUCCESS=1
  export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
  expect_rejected "offline $load_case" \
    --mode="$load_mode" \
    --port=41059 \
    --app-dir="$TEST_ROOT/offline-$load_case-target" \
    --offline="$valid_offline_bundle"
  unset TEST_LOAD_FAILURE_ON TEST_LOAD_NO_TAG_ON TEST_LOAD_SUCCESS TEST_INSTALLER_TMP_ROOT
  if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
    echo "offline $load_case reached container deployment" >&2
    cat "$DOCKER_CALLS" >&2
    exit 1
  fi
  if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
    echo "offline $load_case left a temporary log behind" >&2
    exit 1
  fi
done

for load_case in main-failure main-no-tag updater-failure updater-no-tag; do
  : >"$DOCKER_CALLS"
  load_mode=a
  case "$load_case" in
    main-failure) export TEST_LOAD_FAILURE_ON=1 ;;
    main-no-tag) export TEST_LOAD_NO_TAG_ON=1 ;;
    updater-failure)
      load_mode=b
      export TEST_LOAD_FAILURE_ON=2
      ;;
    updater-no-tag)
      load_mode=b
      export TEST_LOAD_NO_TAG_ON=2
      ;;
  esac
  export TEST_DOWNLOAD_MODE=valid-both
  export TEST_LOAD_SUCCESS=1
  export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
  expect_rejected "online $load_case" \
    --mode="$load_mode" \
    --port=41060 \
    --app-dir="$TEST_ROOT/online-$load_case-target"
  unset TEST_DOWNLOAD_MODE TEST_LOAD_FAILURE_ON TEST_LOAD_NO_TAG_ON TEST_LOAD_SUCCESS TEST_INSTALLER_TMP_ROOT
  if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
    echo "online $load_case reached container deployment" >&2
    cat "$DOCKER_CALLS" >&2
    exit 1
  fi
  if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
    echo "online $load_case left a temporary image or log behind" >&2
    exit 1
  fi
done

: >"$DOCKER_CALLS"
export TEST_MAIN_PULL_SUCCESS=1
expect_rejected "fresh Mode A container launch failure" \
  --mode=a \
  --port=41061 \
  --app-dir="$TEST_ROOT/fresh-mode-a-launch-failure"
unset TEST_MAIN_PULL_SUCCESS
test "$(grep -Ec '^run -d ' "$DOCKER_CALLS")" -eq 1
if grep -Eq '(^| )(stop|rm|compose down)( |$)' "$DOCKER_CALLS"; then
  echo "fresh Mode A launch failure attempted rollback of a previous install" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
export TEST_ALL_PULL_SUCCESS=1
expect_rejected "fresh Mode B compose launch failure" \
  --mode=b \
  --port=41062 \
  --app-dir="$TEST_ROOT/fresh-mode-b-launch-failure"
unset TEST_ALL_PULL_SUCCESS
test "$(grep -Ec '^compose up -d$' "$DOCKER_CALLS")" -eq 1
if grep -Eq '(^| )(stop|rm|compose down)( |$)' "$DOCKER_CALLS"; then
  echo "fresh Mode B launch failure attempted rollback of a previous install" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
existing_target="$TEST_ROOT/existing-failed-rerun"
mkdir -p "$existing_target/data"
printf '%s\n' 'CLM_PORT=41044' 'SENTINEL_ENV=preserve-me' >"$existing_target/data/.env"
printf '%s\n' 'sentinel database bytes' >"$existing_target/data/clm.db"
printf '%s\n' 'sentinel compose bytes' >"$existing_target/docker-compose.yml"
existing_env_sha=$(sha256sum "$existing_target/data/.env" | awk '{print $1}')
existing_db_sha=$(sha256sum "$existing_target/data/clm.db" | awk '{print $1}')
existing_compose_sha=$(sha256sum "$existing_target/docker-compose.yml" | awk '{print $1}')
export TEST_EXISTING_INSTALL=1
export TEST_DOWNLOAD_MODE=mirror-failure
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "failed rerun over an existing installation" \
  --mode=a \
  --port=41044 \
  --app-dir="$existing_target"
unset TEST_EXISTING_INSTALL TEST_DOWNLOAD_MODE TEST_INSTALLER_TMP_ROOT
if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "failed rerun changed the existing container lifecycle" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
test "$(sha256sum "$existing_target/data/.env" | awk '{print $1}')" = "$existing_env_sha"
test "$(sha256sum "$existing_target/data/clm.db" | awk '{print $1}')" = "$existing_db_sha"
test "$(sha256sum "$existing_target/docker-compose.yml" | awk '{print $1}')" = "$existing_compose_sha"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed rerun left a temporary download behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
export TEST_EXISTING_INSTALL=1
export TEST_DOWNLOAD_MODE=updater-interrupted
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
export TEST_LOAD_SUCCESS=1
export TEST_UPDATER_DOWNLOAD=1
expect_rejected "failed updater download over an existing installation" \
  --mode=b \
  --port=41044 \
  --app-dir="$existing_target"
unset TEST_EXISTING_INSTALL TEST_DOWNLOAD_MODE TEST_INSTALLER_TMP_ROOT TEST_LOAD_SUCCESS TEST_UPDATER_DOWNLOAD
if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "failed updater download changed the existing container lifecycle" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
test "$(grep -Ec '^load($| -i )' "$DOCKER_CALLS")" -eq 1
test "$(sha256sum "$existing_target/data/.env" | awk '{print $1}')" = "$existing_env_sha"
test "$(sha256sum "$existing_target/data/clm.db" | awk '{print $1}')" = "$existing_db_sha"
test "$(sha256sum "$existing_target/docker-compose.yml" | awk '{print $1}')" = "$existing_compose_sha"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed updater download left a temporary file behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
export TEST_EXISTING_INSTALL=1
expect_rejected "failed registry pull over an existing installation" \
  --mode=a \
  --port=41044 \
  --app-dir="$existing_target"
unset TEST_EXISTING_INSTALL
if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "failed registry pull changed the existing container lifecycle" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
grep -Eq '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"
test "$(sha256sum "$existing_target/data/.env" | awk '{print $1}')" = "$existing_env_sha"
test "$(sha256sum "$existing_target/data/clm.db" | awk '{print $1}')" = "$existing_db_sha"
test "$(sha256sum "$existing_target/docker-compose.yml" | awk '{print $1}')" = "$existing_compose_sha"

: >"$DOCKER_CALLS"
export TEST_EXISTING_INSTALL=1
export TEST_MAIN_PULL_SUCCESS=1
expect_rejected "failed updater registry pull over an existing installation" \
  --mode=b \
  --port=41044 \
  --app-dir="$existing_target"
unset TEST_EXISTING_INSTALL TEST_MAIN_PULL_SUCCESS
if grep -Eq '(^| )(stop|rm|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "failed updater registry pull changed the existing container lifecycle" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
grep -Eq '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"
grep -Eq '^pull ghcr.io/clubfoundry/updater:latest$' "$DOCKER_CALLS"
test "$(sha256sum "$existing_target/data/.env" | awk '{print $1}')" = "$existing_env_sha"
test "$(sha256sum "$existing_target/data/clm.db" | awk '{print $1}')" = "$existing_db_sha"
test "$(sha256sum "$existing_target/docker-compose.yml" | awk '{print $1}')" = "$existing_compose_sha"

: >"$DOCKER_CALLS"
export TEST_DOWNLOAD_MODE=updater-interrupted
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
export TEST_LOAD_SUCCESS=1
export TEST_UPDATER_DOWNLOAD=1
expect_rejected "interrupted updater download" \
  --mode=b \
  --port=41043 \
  --app-dir="$TEST_ROOT/updater-interrupted"
unset TEST_DOWNLOAD_MODE TEST_INSTALLER_TMP_ROOT TEST_LOAD_SUCCESS TEST_UPDATER_DOWNLOAD
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "interrupted updater download left a temporary file behind" >&2
  exit 1
fi
test "$(grep -Ec '^load($| -i )' "$DOCKER_CALLS")" -eq 1
if grep -Eq '(^| )(stop|rm|pull|run|up|down)( |$)' "$DOCKER_CALLS"; then
  echo "interrupted updater download reached container deployment" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
successful_rerun_target="$TEST_ROOT/existing-successful-rerun"
mkdir -p "$successful_rerun_target/data"
printf '%s\n' 'CLM_PORT=41045' 'SENTINEL_ENV=replace-after-pull' >"$successful_rerun_target/data/.env"
printf '%s\n' 'preserved database bytes' >"$successful_rerun_target/data/clm.db"
printf '%s\n' 'sentinel compose bytes' >"$successful_rerun_target/docker-compose.yml"
successful_rerun_db_sha=$(sha256sum "$successful_rerun_target/data/clm.db" | awk '{print $1}')
export TEST_EXISTING_INSTALL=1
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
bash "$INSTALLER" \
  --mode=a \
  --port=41045 \
  --app-dir="$successful_rerun_target" >/dev/null 2>&1
unset TEST_EXISTING_INSTALL TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN TEST_INSTALLER_TMP_ROOT
pull_line=$(grep -n '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS" | cut -d: -f1)
stop_line=$(grep -n '^stop clubfoundry clubfoundry-updater$' "$DOCKER_CALLS" | cut -d: -f1)
run_line=$(grep -n '^run -d ' "$DOCKER_CALLS" | cut -d: -f1)
if [ -z "$pull_line" ] || [ -z "$stop_line" ] || [ -z "$run_line" ] \
  || [ "$pull_line" -ge "$stop_line" ] || [ "$stop_line" -ge "$run_line" ]; then
  echo "successful rerun did not prepare, stop, and replace in order" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
test "$(sha256sum "$successful_rerun_target/data/clm.db" | awk '{print $1}')" = "$successful_rerun_db_sha"
grep -Fqx 'CLM_PORT=41045' "$successful_rerun_target/data/.env"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "successful rerun left a temporary rollback backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
failed_deploy_target="$TEST_ROOT/existing-failed-deploy"
mkdir -p "$failed_deploy_target/data"
printf '%s\n' 'CLM_PORT=41047' 'SENTINEL_ENV=restore-after-failed-run' >"$failed_deploy_target/data/.env"
printf '%s\n' 'preserved database after failed run' >"$failed_deploy_target/data/clm.db"
printf '%s\n' 'cflink_existing_token_must_be_restored' >"$failed_deploy_target/data/.link-token"
chmod 640 "$failed_deploy_target/data/.env"
chmod 600 "$failed_deploy_target/data/.link-token"
failed_deploy_env_sha=$(sha256sum "$failed_deploy_target/data/.env" | awk '{print $1}')
failed_deploy_db_sha=$(sha256sum "$failed_deploy_target/data/clm.db" | awk '{print $1}')
failed_deploy_link_sha=$(sha256sum "$failed_deploy_target/data/.link-token" | awk '{print $1}')
failed_deploy_env_meta=$(stat -c '%a:%u:%g' "$failed_deploy_target/data/.env")
failed_deploy_link_meta=$(stat -c '%a:%u:%g' "$failed_deploy_target/data/.link-token")
export TEST_EXISTING_INSTALL=1
export TEST_REPLACEMENT_RUN_FAILURE=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "replacement run failure over an existing installation" \
  --mode=a \
  --port=41047 \
  --app-dir="$failed_deploy_target" \
  --link-token=cflink_12345678901234567890
unset TEST_EXISTING_INSTALL TEST_REPLACEMENT_RUN_FAILURE TEST_INSTALLER_TMP_ROOT
grep -Eq '^run -d .*ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"
if ! grep -Eq '^run -d .*ghcr.io/clubfoundry/clubfoundry:1.2.3$' "$DOCKER_CALLS"; then
  echo "failed replacement did not restart the previous image" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
if [ "$(sha256sum "$failed_deploy_target/data/.env" | awk '{print $1}')" != "$failed_deploy_env_sha" ]; then
  echo "failed replacement did not restore the previous environment" >&2
  exit 1
fi
test "$(sha256sum "$failed_deploy_target/data/clm.db" | awk '{print $1}')" = "$failed_deploy_db_sha"
test "$(sha256sum "$failed_deploy_target/data/.link-token" | awk '{print $1}')" = "$failed_deploy_link_sha"
test "$(stat -c '%a:%u:%g' "$failed_deploy_target/data/.env")" = "$failed_deploy_env_meta"
test "$(stat -c '%a:%u:%g' "$failed_deploy_target/data/.link-token")" = "$failed_deploy_link_meta"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed replacement rollback left a temporary backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
failed_rollback_target="$TEST_ROOT/existing-failed-rollback"
mkdir -p "$failed_rollback_target/data"
printf '%s\n' 'CLM_PORT=41048' 'SENTINEL_ENV=preserve-when-rollback-run-fails' >"$failed_rollback_target/data/.env"
printf '%s\n' 'preserved database when rollback run fails' >"$failed_rollback_target/data/clm.db"
failed_rollback_env_sha=$(sha256sum "$failed_rollback_target/data/.env" | awk '{print $1}')
failed_rollback_db_sha=$(sha256sum "$failed_rollback_target/data/clm.db" | awk '{print $1}')
export TEST_EXISTING_INSTALL=1
export TEST_REPLACEMENT_RUN_FAILURE=1
export TEST_ROLLBACK_RUN_FAILURE=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "replacement and rollback run failure" \
  --mode=a \
  --port=41048 \
  --app-dir="$failed_rollback_target"
unset TEST_EXISTING_INSTALL TEST_REPLACEMENT_RUN_FAILURE TEST_ROLLBACK_RUN_FAILURE TEST_INSTALLER_TMP_ROOT
test "$(grep -Ec '^run -d ' "$DOCKER_CALLS")" -eq 2
test "$(sha256sum "$failed_rollback_target/data/.env" | awk '{print $1}')" = "$failed_rollback_env_sha"
test "$(sha256sum "$failed_rollback_target/data/clm.db" | awk '{print $1}')" = "$failed_rollback_db_sha"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed rollback run left a temporary backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
failed_compose_target="$TEST_ROOT/existing-failed-compose"
mkdir -p "$failed_compose_target/data"
printf '%s\n' 'CLM_PORT=41049' 'SENTINEL_ENV=restore-after-failed-compose' >"$failed_compose_target/data/.env"
printf '%s\n' 'preserved database after failed compose' >"$failed_compose_target/data/clm.db"
printf '%s\n' 'cflink_existing_compose_token' >"$failed_compose_target/data/.link-token"
printf '%s\n' 'services:' '  clubfoundry:' '    image: ghcr.io/clubfoundry/clubfoundry:1.2.3' >"$failed_compose_target/docker-compose.yml"
chmod 640 "$failed_compose_target/data/.env"
chmod 600 "$failed_compose_target/data/.link-token"
chmod 640 "$failed_compose_target/docker-compose.yml"
failed_compose_env_sha=$(sha256sum "$failed_compose_target/data/.env" | awk '{print $1}')
failed_compose_db_sha=$(sha256sum "$failed_compose_target/data/clm.db" | awk '{print $1}')
failed_compose_link_sha=$(sha256sum "$failed_compose_target/data/.link-token" | awk '{print $1}')
failed_compose_file_sha=$(sha256sum "$failed_compose_target/docker-compose.yml" | awk '{print $1}')
failed_compose_env_meta=$(stat -c '%a:%u:%g' "$failed_compose_target/data/.env")
failed_compose_link_meta=$(stat -c '%a:%u:%g' "$failed_compose_target/data/.link-token")
failed_compose_file_meta=$(stat -c '%a:%u:%g' "$failed_compose_target/docker-compose.yml")
export TEST_EXISTING_INSTALL=1
export TEST_REPLACEMENT_COMPOSE_FAILURE=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "replacement compose failure over an existing installation" \
  --mode=b \
  --port=41049 \
  --app-dir="$failed_compose_target" \
  --link-token=cflink_12345678901234567890
unset TEST_EXISTING_INSTALL TEST_REPLACEMENT_COMPOSE_FAILURE TEST_INSTALLER_TMP_ROOT
test "$(grep -Ec '^compose up -d$' "$DOCKER_CALLS")" -eq 2
test "$(sha256sum "$failed_compose_target/data/.env" | awk '{print $1}')" = "$failed_compose_env_sha"
test "$(sha256sum "$failed_compose_target/data/clm.db" | awk '{print $1}')" = "$failed_compose_db_sha"
test "$(sha256sum "$failed_compose_target/data/.link-token" | awk '{print $1}')" = "$failed_compose_link_sha"
test "$(sha256sum "$failed_compose_target/docker-compose.yml" | awk '{print $1}')" = "$failed_compose_file_sha"
test "$(stat -c '%a:%u:%g' "$failed_compose_target/data/.env")" = "$failed_compose_env_meta"
test "$(stat -c '%a:%u:%g' "$failed_compose_target/data/.link-token")" = "$failed_compose_link_meta"
test "$(stat -c '%a:%u:%g' "$failed_compose_target/docker-compose.yml")" = "$failed_compose_file_meta"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed compose rollback left a temporary backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
failed_compose_rollback_target="$TEST_ROOT/existing-failed-compose-rollback"
mkdir -p "$failed_compose_rollback_target/data"
printf '%s\n' 'CLM_PORT=41050' 'SENTINEL_ENV=preserve-when-compose-rollback-fails' >"$failed_compose_rollback_target/data/.env"
printf '%s\n' 'preserved database when compose rollback fails' >"$failed_compose_rollback_target/data/clm.db"
printf '%s\n' 'services:' '  clubfoundry:' '    image: ghcr.io/clubfoundry/clubfoundry:1.2.3' >"$failed_compose_rollback_target/docker-compose.yml"
chmod 640 "$failed_compose_rollback_target/data/.env"
chmod 640 "$failed_compose_rollback_target/docker-compose.yml"
failed_compose_rollback_env_sha=$(sha256sum "$failed_compose_rollback_target/data/.env" | awk '{print $1}')
failed_compose_rollback_db_sha=$(sha256sum "$failed_compose_rollback_target/data/clm.db" | awk '{print $1}')
failed_compose_rollback_file_sha=$(sha256sum "$failed_compose_rollback_target/docker-compose.yml" | awk '{print $1}')
failed_compose_rollback_env_meta=$(stat -c '%a:%u:%g' "$failed_compose_rollback_target/data/.env")
failed_compose_rollback_file_meta=$(stat -c '%a:%u:%g' "$failed_compose_rollback_target/docker-compose.yml")
export TEST_EXISTING_INSTALL=1
export TEST_REPLACEMENT_COMPOSE_FAILURE=1
export TEST_ROLLBACK_COMPOSE_FAILURE=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
expect_rejected "replacement and rollback compose failure" \
  --mode=b \
  --port=41050 \
  --app-dir="$failed_compose_rollback_target" \
  --link-token=cflink_12345678901234567890
unset TEST_EXISTING_INSTALL TEST_REPLACEMENT_COMPOSE_FAILURE TEST_ROLLBACK_COMPOSE_FAILURE TEST_INSTALLER_TMP_ROOT
test "$(grep -Ec '^compose up -d$' "$DOCKER_CALLS")" -eq 2
test "$(sha256sum "$failed_compose_rollback_target/data/.env" | awk '{print $1}')" = "$failed_compose_rollback_env_sha"
test "$(sha256sum "$failed_compose_rollback_target/data/clm.db" | awk '{print $1}')" = "$failed_compose_rollback_db_sha"
test "$(sha256sum "$failed_compose_rollback_target/docker-compose.yml" | awk '{print $1}')" = "$failed_compose_rollback_file_sha"
test "$(stat -c '%a:%u:%g' "$failed_compose_rollback_target/data/.env")" = "$failed_compose_rollback_env_meta"
test "$(stat -c '%a:%u:%g' "$failed_compose_rollback_target/docker-compose.yml")" = "$failed_compose_rollback_file_meta"
test ! -e "$failed_compose_rollback_target/data/.link-token"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "failed compose rollback run left a temporary backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
mode_b_target="$TEST_ROOT/mode-b-registry-success"
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
bash "$INSTALLER" \
  --mode=b \
  --port=41046 \
  --app-dir="$mode_b_target" >/dev/null 2>&1
unset TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN
grep -Eq '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"
grep -Eq '^pull ghcr.io/clubfoundry/updater:latest$' "$DOCKER_CALLS"
grep -Eq '^compose up -d$' "$DOCKER_CALLS"
grep -Fq 'image: ghcr.io/clubfoundry/clubfoundry:latest' "$mode_b_target/docker-compose.yml"
grep -Fq 'image: ghcr.io/clubfoundry/updater:latest' "$mode_b_target/docker-compose.yml"
grep -Fqx 'CLM_PORT=41046' "$mode_b_target/data/.env"

: >"$DOCKER_CALLS"
mode_b_rerun_target="$TEST_ROOT/mode-b-successful-rerun"
mkdir -p "$mode_b_rerun_target/data"
printf '%s\n' 'CLM_PORT=41051' 'SENTINEL_ENV=replace-on-update' >"$mode_b_rerun_target/data/.env"
printf '%s\n' 'database bytes preserved across repeated updates' >"$mode_b_rerun_target/data/clm.db"
printf '%s\n' 'services:' '  clubfoundry:' '    image: ghcr.io/clubfoundry/clubfoundry:1.2.3' >"$mode_b_rerun_target/docker-compose.yml"
mode_b_rerun_db_sha=$(sha256sum "$mode_b_rerun_target/data/clm.db" | awk '{print $1}')
export TEST_EXISTING_INSTALL=1
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
bash "$INSTALLER" \
  --update \
  --mode=b \
  --app-dir="$mode_b_rerun_target" >/dev/null 2>&1
first_rerun_env_sha=$(sha256sum "$mode_b_rerun_target/data/.env" | awk '{print $1}')
first_rerun_compose_sha=$(sha256sum "$mode_b_rerun_target/docker-compose.yml" | awk '{print $1}')
bash "$INSTALLER" \
  --update \
  --mode=b \
  --app-dir="$mode_b_rerun_target" >/dev/null 2>&1
unset TEST_EXISTING_INSTALL TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN TEST_INSTALLER_TMP_ROOT
test "$(sha256sum "$mode_b_rerun_target/data/clm.db" | awk '{print $1}')" = "$mode_b_rerun_db_sha"
test "$(sha256sum "$mode_b_rerun_target/data/.env" | awk '{print $1}')" = "$first_rerun_env_sha"
test "$(sha256sum "$mode_b_rerun_target/docker-compose.yml" | awk '{print $1}')" = "$first_rerun_compose_sha"
grep -Fqx 'CLM_PORT=41051' "$mode_b_rerun_target/data/.env"
grep -Fq 'image: ghcr.io/clubfoundry/clubfoundry:latest' "$mode_b_rerun_target/docker-compose.yml"
grep -Fq 'image: ghcr.io/clubfoundry/updater:latest' "$mode_b_rerun_target/docker-compose.yml"
test "$(grep -Ec '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS")" -eq 2
test "$(grep -Ec '^pull ghcr.io/clubfoundry/updater:latest$' "$DOCKER_CALLS")" -eq 2
test "$(grep -Ec '^compose down --remove-orphans$' "$DOCKER_CALLS")" -eq 2
test "$(grep -Ec '^compose up -d$' "$DOCKER_CALLS")" -eq 2
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "successful repeated Mode B update left a temporary rollback backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
legacy_compose_target="$TEST_ROOT/legacy-compose-successful-update"
mkdir -p "$legacy_compose_target/data"
printf '%s\n' 'CLM_PORT=41052' >"$legacy_compose_target/data/.env"
printf '%s\n' 'legacy compose database bytes' >"$legacy_compose_target/data/clm.db"
printf '%s\n' 'services:' '  clubfoundry:' '    image: ghcr.io/clubfoundry/clubfoundry:1.2.3' >"$legacy_compose_target/docker-compose.yml"
legacy_compose_db_sha=$(sha256sum "$legacy_compose_target/data/clm.db" | awk '{print $1}')
export TEST_COMPOSE_PLUGIN_MISSING=1
export TEST_EXISTING_INSTALL=1
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
bash "$INSTALLER" \
  --update \
  --mode=b \
  --app-dir="$legacy_compose_target" >/dev/null 2>&1
unset TEST_COMPOSE_PLUGIN_MISSING TEST_EXISTING_INSTALL TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN TEST_INSTALLER_TMP_ROOT
test "$(sha256sum "$legacy_compose_target/data/clm.db" | awk '{print $1}')" = "$legacy_compose_db_sha"
grep -Fqx 'CLM_PORT=41052' "$legacy_compose_target/data/.env"
grep -Fqx 'legacy-compose down --remove-orphans' "$DOCKER_CALLS"
grep -Fqx 'legacy-compose up -d' "$DOCKER_CALLS"
if grep -Eq '^compose (down|up)' "$DOCKER_CALLS"; then
  echo "legacy docker-compose update unexpectedly used the Compose plugin" >&2
  cat "$DOCKER_CALLS" >&2
  exit 1
fi
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "legacy docker-compose update left a temporary rollback backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
legacy_env_update_target="$TEST_ROOT/legacy-env-successful-update"
mkdir -p "$legacy_env_update_target/data"
printf '%s\n' 'LEGACY_SETTING=old-install-without-port' >"$legacy_env_update_target/data/.env"
printf '%s\n' 'legacy environment database bytes' >"$legacy_env_update_target/data/clm.db"
printf '%s\n' 'services:' '  clubfoundry:' '    image: ghcr.io/clubfoundry/clubfoundry:1.2.3' >"$legacy_env_update_target/docker-compose.yml"
legacy_env_update_db_sha=$(sha256sum "$legacy_env_update_target/data/clm.db" | awk '{print $1}')
export TEST_EXISTING_INSTALL=1
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
export TEST_INSTALLER_TMP_ROOT="$TEST_ROOT"
bash "$INSTALLER" \
  --update \
  --mode=b \
  --app-dir="$legacy_env_update_target" >/dev/null 2>&1
unset TEST_EXISTING_INSTALL TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN TEST_INSTALLER_TMP_ROOT
test "$(sha256sum "$legacy_env_update_target/data/clm.db" | awk '{print $1}')" = "$legacy_env_update_db_sha"
grep -Fqx 'CLM_PORT=3000' "$legacy_env_update_target/data/.env"
grep -Fq 'image: ghcr.io/clubfoundry/clubfoundry:latest' "$legacy_env_update_target/docker-compose.yml"
grep -Fq 'image: ghcr.io/clubfoundry/updater:latest' "$legacy_env_update_target/docker-compose.yml"
grep -Fqx 'compose down --remove-orphans' "$DOCKER_CALLS"
grep -Fqx 'compose up -d' "$DOCKER_CALLS"
if compgen -G "$TEST_ROOT/installer-tmp.*" >/dev/null; then
  echo "legacy environment update left a temporary rollback backup behind" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
export TEST_DEPLOY_SUCCESS=1
export TEST_FAST_SLEEP=1
export TEST_HEALTH_DOWN=1
health_output=$(bash "$INSTALLER" \
  --mode=a \
  --port=41042 \
  --app-dir="$TEST_ROOT/unhealthy-start" 2>&1)
unset TEST_DEPLOY_SUCCESS TEST_FAST_SLEEP TEST_HEALTH_DOWN
printf '%s\n' "$health_output" | grep -Fq "ClubFoundry may still be starting"
printf '%s\n' "$health_output" | grep -Fq "ClubFoundry installed successfully"
grep -Eq '^pull ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"
grep -Eq '^run -d .*ghcr.io/clubfoundry/clubfoundry:latest$' "$DOCKER_CALLS"

echo "installer contracts: OK"
