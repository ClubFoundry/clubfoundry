# Sidecar Third-Party Notices

This file records third-party software present in the ClubFoundry updater
sidecar. It is an attribution and audit record, not a legal conclusion.

## Go standard library

The sidecar has no third-party Go modules. `go list -m all` contains only
`github.com/clubfoundry/updater`; the compiled binary uses the Go standard
library.

- Component: The Go Programming Language standard library
- Copyright: Copyright 2009 The Go Authors
- License: BSD-3-Clause
- License text: https://go.dev/LICENSE

The Go license must accompany binary distributions in their documentation or
other materials.

## Runtime image

The runtime image is built from `alpine:3.24` and directly installs
`ca-certificates` and `tzdata`. Their transitive runtime packages are recorded
in `runtime-packages.lock` as `package|version|SPDX-expression`. Docker CLI and
Docker Compose are copied from Docker's official multi-architecture images
pinned by OCI index digest. Their versions, digests, and licenses are recorded
in `runtime-tools.lock`.

The package lock was generated from `/lib/apk/db/installed` in an amd64 image
built on 2026-08-11. `audit-runtime-licenses.sh` reproduces the inventory for
any built image. Release CI must fail when the generated inventory or pinned
Docker tool version differs from its lock, so package or license drift receives
an explicit review.

Primary upstream sources include:

- Alpine packages: https://pkgs.alpinelinux.org/packages?branch=v3.24
- Alpine Package Keeper: https://gitlab.alpinelinux.org/alpine/apk-tools
- BusyBox: https://busybox.net/source.html
- Docker CLI: https://github.com/docker/cli
- Docker Compose: https://github.com/docker/compose
- musl libc: https://musl.libc.org/
- OpenSSL: https://github.com/openssl/openssl
- pax-utils: https://gitweb.gentoo.org/proj/pax-utils.git/
- Time Zone Database: https://www.iana.org/time-zones
- zlib: https://zlib.net/

## Distribution evidence

The image includes the applicable standard license texts under `/licenses`, this
notice at `/THIRD_PARTY_NOTICES.md`, and the corresponding-source offer at
`/SOURCE_OFFER.md`. `LICENSES/license-texts.lock` pins the copied SPDX 3.28.0
texts by SHA-256. The release workflow rejects runtime package or license-text
drift, attaches a machine-readable SBOM and provenance to the immutable image
digest, and exports a checksummed evidence bundle.

These technical controls do not replace legal review of the exact release.
