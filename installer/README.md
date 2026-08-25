# ClubFoundry Installer Source

[Русская версия](README.ru.md) | [Project overview](../README.md) | [Install the ClubFoundry application](../INSTALLATION.md)

> This page is technical documentation for the public source of
> `installer/install.sh`, not the user installation guide for the ClubFoundry
> application. To install ClubFoundry on TrueNAS SCALE or another Linux Docker
> host, follow the [application installation guide](../INSTALLATION.md).

This directory contains the public source of the ClubFoundry installer. It
preflights the host, acquires integrity-checked application images, selects
persistent storage, writes the Docker Compose configuration, and starts the
main application with the optional updater sidecar.

`installer/install.sh` is the canonical public installer source.

## Running the installer

Download the script before running it so it can be inspected:

```bash
curl -fL \
  https://raw.githubusercontent.com/ClubFoundry/clubfoundry/main/installer/install.sh \
  -o install.sh
less install.sh
sudo bash install.sh
```

A fresh interactive installation asks whether this instance is for personal
non-commercial use or commercial/professional use. For automation, specify the
choice explicitly:

```bash
sudo bash install.sh --usage-mode=noncommercial
sudo bash install.sh --usage-mode=commercial
```

TrueNAS API credentials are entered in the browser during first-run setup, not
as ordinary public command-line arguments.

## Deployment modes

The recommended mode is the application plus the `clubfoundry-updater`
sidecar managed by Docker Compose. The sidecar provides verified updates,
rollback, recovery, and crash-loop protection.

A single-container fallback is selected when Compose is unavailable or can be
forced with `--mode=a`. That mode has no updater sidecar.

The web UI defaults to port 3000 and falls back to another free port if needed.
Override it with `--port=N`.

## Persistent data

On TrueNAS, the installer prefers the data pool selected for Apps. With no Apps
pool, it auto-selects only when exactly one suitable mounted ZFS data pool is
available. If there are several pools, specify the destination:

```bash
sudo bash install.sh --app-dir=/mnt/POOL/clubfoundry
```

On another Linux Docker host, always pass a persistent absolute path such as
`--app-dir=/srv/clubfoundry`.

## Offline GitHub Release bundle

The self-contained GitHub Release bundle uses the same installer:

```bash
tar -xzf clubfoundry-VERSION-offline.tar.gz
cd clubfoundry-VERSION-offline
sudo bash install.sh --offline="$PWD"
```

Before loading either image, the installer verifies the bundle's `MANIFEST`,
`install.sh`, and image archives against its internal `SHA256SUMS`.

## Other options

- `--channel=stable|beta|lts`
- `--link-token=cflink_...`
- `--offline=/absolute/path/to/bundle`
- `--app-dir=/absolute/persistent/path`
- `--port=N`
- `--usage-mode=commercial|noncommercial`
- `--update`
- `--dry-run`
- `--help`

Unknown options fail before host changes. `--dry-run` prints the resolved plan
without writing files, loading images, or changing containers.

`--clean-install` is intentionally destructive: it removes the existing
ClubFoundry database and state after verifying ownership of same-named
containers. It must not be used as a normal update command.

## Update

```bash
sudo bash install.sh --update
```

An offline update uses the installer from the newer bundle:

```bash
sudo bash NEW-BUNDLE/install.sh --update --offline=/absolute/path/to/NEW-BUNDLE
```

## Testing

Fast checks do not touch Docker or TrueNAS:

```bash
bash -n installer/install.sh installer/test-fresh-install-e2e.sh
sh -n installer/bootstrap.sh
bash installer/install.sh --help
bash installer/test-installer-contracts.sh
```

`test-fresh-install-e2e.sh` is a privileged test for an explicitly configured
disposable TrueNAS system. Review its header and environment variables before
running it. Never point it at a customer system.

## License

MIT; see [`LICENSE.md`](LICENSE.md). The compiled main application and Windows
Agent have a different licensing boundary described in the repository's
[`LICENSE.md`](../LICENSE.md).
