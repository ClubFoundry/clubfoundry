#!/usr/bin/env bash
set -euo pipefail

image="${1:?Usage: audit-runtime-licenses.sh IMAGE}"

docker run --rm --entrypoint /bin/sh "$image" -c '
  awk -F: '\''
    /^P:/ { name = substr($0, 3) }
    /^V:/ { version = substr($0, 3) }
    /^L:/ { license = substr($0, 3) }
    /^$/ {
      if (name != "") print name "|" version "|" license
      name = version = license = ""
    }
    END {
      if (name != "") print name "|" version "|" license
    }
  '\'' /lib/apk/db/installed | sort
'
