# ClubFoundry Sidecar

[Русская версия](README.ru.md) | [Project overview](../README.md)

The sidecar is a companion container for standard Docker Compose installations
of ClubFoundry. In addition to verified main-application updates and stepped
upgrades, it provides rollback, self-update, crash recovery, crash-loop
protection, and diagnostic collection.

The online installer deploys the sidecar automatically. The ready-made offline
bundle in GitHub Releases already contains both the main-application and sidecar
images and installs them together in the standard Docker Compose mode. The
sidecar does not need to be downloaded or installed separately.

ClubFoundry is not currently published in the TrueNAS Apps catalog. The code
already provides a mode for a future catalog installation; in that configuration
the sidecar does not run because the catalog owns the image lifecycle.

- Image: `ghcr.io/clubfoundry/updater`
- Runtime: Linux container, amd64 and arm64
- API: HTTP on loopback port `3001`; `/health` is public and every other endpoint
  requires the bearer token stored in the shared data directory
- Toolchain: Go 1.26.5, standard library only
- Runtime tools: Alpine 3.24 plus digest-pinned official Docker CLI 29.5.3 and Compose 5.4.0
- License: MIT; see [`LICENSE.md`](LICENSE.md)
- Release evidence: `RELEASE_EVIDENCE.md`

## Verify and build

Run the fast source checks without Docker:

```bash
cd sidecar
test -z "$(gofmt -l .)"
go vet ./...
go test -race -timeout 2m ./...
go build -trimpath -ldflags "-X main.version=v0.LOCAL" ./cmd/updater
```

Build the runtime image from the repository root. `VERSION` is mandatory because
self-update validates the compiled version after recreating the container.

```bash
docker build \
  --build-arg VERSION=v0.LOCAL \
  -t ghcr.io/clubfoundry/updater:v0.LOCAL \
  sidecar/
```

The runtime image contains Docker CLI and Compose. It needs read-write access to
the host Docker socket and to the managed Compose file, so it must be deployed by
the installer-generated stack rather than as an isolated container.
The image also contains third-party notices, pinned standard license texts, and
the corresponding-source offer. Run `python sidecar/verify-release-evidence.py`
from the repository root to verify that bundle without Docker.

## Docker privilege boundary

Read-write access to `/var/run/docker.sock` grants control of the host Docker
daemon and must be treated as host-level privilege. The sidecar needs it to run
Compose lifecycle commands, load and tag verified release images, remove retained
images, run disposable migration checks, and replace its own container.

The configured Compose directory and service names define the managed stack.
Lifecycle commands target those services, and image retention scans only the
configured ClubFoundry repositories while preserving images used by any
container. Exact container names are reserved. Conflict recovery inspects a
same-named container and refuses to remove it unless its image matches the
ClubFoundry ownership rule used by the installer.

BuildKit cache is daemon-wide rather than repository-scoped. When enabled, the
scheduled cleanup uses `docker buildx prune`; operators sharing the daemon with
other build workloads can disable it with `auto_prune_buildcache_opt_out`.
The mode reserved for a future TrueNAS Apps catalog installation does not run
the sidecar and therefore does not grant it Docker socket access.

## Artifact trust boundary

Multi-mirror downloads are written to isolated temporary files and must match the
published SHA-256 before `docker load` runs. Registry fallback uses Docker's
registry verification, and an image already present under the requested local tag
is trusted as local daemon state instead of being downloaded again.

Single-URL downloads use the same verification-before-import boundary. The
artifact is written to an isolated temporary file, flushed and closed, and its
SHA-256 must match before `docker load` starts. The temporary file is removed on
success and on every failure path.

## Platform contract

The supported runtime is a Linux container on amd64 or arm64. Docker CLI,
Compose, the Docker socket at `/var/run/docker.sock`, and a writable managed
Compose file are required. Disk-space probes and ownership preservation use
Linux system calls behind build tags. Non-Linux builds exist only for local
development and tests; unsupported probes return an explicit unavailable result
instead of guessing host behavior.

## Architecture

```text
sidecar/
|-- cmd/updater/             process startup and dependency wiring
|-- internal/auth/           local bearer-token authentication
|-- internal/backup/         SQLite database, WAL, and SHM backup/restore
|-- internal/bootstrap/      legacy-install reconciliation
|-- internal/cloud/          update control-plane and mirror clients
|-- internal/config/         persisted sidecar settings
|-- internal/dockerops/      constrained Docker and Compose operations
|-- internal/footprint/      image and data-volume accounting
|-- internal/handlers/       authenticated HTTP surface
|-- internal/health/         main-application health probes
|-- internal/history/        update attempt history
|-- internal/monitor/        crash-loop detection and recovery
|-- internal/poller/         scheduled update and recall decisions
|-- internal/state/          persisted operation state machines
|-- internal/telemetry/      bounded operational telemetry
|-- internal/updater/        update, rollback, and self-update orchestration
|-- LICENSES/                pinned standard third-party license texts
|-- RELEASE_EVIDENCE.md      build attestation and verification contract
|-- runtime-tools.lock       digest-pinned non-APK runtime tools
`-- Dockerfile               static Go build and Alpine runtime image
```

## HTTP surface

The main application is the API client. The sidecar exposes endpoint groups for:

- health, status, configuration, footprint, logs, history, and diagnostics;
- preflight, stage, apply, cancel, reset, update, and rollback;
- sidecar self-update and recovery artifacts.

`GET /health` is intentionally unauthenticated for container health checks. Every
other endpoint requires `Authorization: Bearer <token>`. Image-lifecycle requests
return `409 CATALOG_MANAGED` when `CLM_UPDATE_MODE=truenas_apps`.

## Safety invariants

1. Direct-download artifacts must match the published SHA-256 before image import.
2. The current image is retained until the target version passes its health gate.
3. Database backup and restore include `clm.db`, `clm.db-wal`, and `clm.db-shm`.
4. Restore preserves the existing database ownership on Linux.
5. Stepped rollback returns to the last successful step.
6. Main-application and self-update operations have separate persisted state.
7. Compose lifecycle operations target the configured ClubFoundry services and
   project; image retention is limited to configured repositories and skips
   images referenced by any container.
8. Catalog-managed installations cannot mutate the application lifecycle through
   the sidecar.

## Configuration

The installer supplies the runtime configuration. The main operator-facing
variables are:

| Variable | Purpose |
|---|---|
| `CLUBFOUNDRY_UPDATER_ADDR` | HTTP bind address; installer uses loopback port 3001. |
| `CLUBFOUNDRY_DATA_DIR` | Shared state and application data directory. |
| `CLUBFOUNDRY_CLOUD_URL` | Update control-plane base URL. |
| `CLUBFOUNDRY_COMPOSE_DIR` | Directory containing the managed Compose file. |
| `CLUBFOUNDRY_HEALTH_URL` | Main-application health endpoint. |
| `CLM_UPDATE_MODE` | `standalone` or `truenas_apps`; catalog mode rejects image-lifecycle mutations. |

Additional variables are implementation-level recovery and test controls. Review
their call sites before overriding installer defaults.

The installer binds the API to `127.0.0.1:3001`. If
`CLUBFOUNDRY_UPDATER_ADDR` is absent, the binary fallback is `:3001`, which
listens on all interfaces; do not remove the installer-provided loopback setting.

## Contributing

Keep behavior changes separate from readability refactors. Comments must be in
English and explain invariants or non-obvious failure modes. Run the verification
commands above before submitting a change.
