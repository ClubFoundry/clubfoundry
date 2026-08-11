#!/bin/sh
echo "[ClubFoundry] picking fastest mirror..."
TMP=$(mktemp /tmp/cf-install.XXXXXX.sh) || exit 1
cleanup() {
  rm -f "$TMP"
}
trap cleanup EXIT INT TERM

for M in https://clubfoundry.net https://mirror.clubfoundry.ru; do
  curl -fsSL -m 3 -I "$M/install.sh" -o /dev/null 2>/dev/null || continue
  echo "[ClubFoundry] using $M"
  curl -fsSL --speed-limit 10000 --speed-time 3 -m 30 "$M/install.sh" -o "$TMP" || continue
  bash "$TMP" "$@"
  exit $?
done
echo "[ClubFoundry] no mirror reachable" >&2
exit 1
