# Installing ClubFoundry

[Русская версия](INSTALLATION.ru.md) | [Project overview](README.md)

The simplest installation path on TrueNAS SCALE is:

1. Open the TrueNAS web interface.
2. Go to `System > Shell`.
3. Paste the command below and press Enter.

```bash
curl -fsSL https://raw.githubusercontent.com/ClubFoundry/clubfoundry/main/installer/install.sh | sudo bash
```

For step-by-step TrueNAS preparation and an explanation of each action, use the
[simple website installation guide](https://clubfoundry.net/install).
Experienced administrators can run the same command over SSH.

The rest of this document describes the requirements and manual installation
paths using GitHub release files.

## Requirements

- TrueNAS SCALE 24.10 or newer with Docker/Apps enabled, or another Linux host
  with a running Docker daemon;
- root or `sudo` access;
- persistent storage. On a non-TrueNAS host, pass an explicit
  `--app-dir=/absolute/path/clubfoundry`.

No additional tools need to be installed on a supported TrueNAS SCALE version:
everything required for the normal `System > Shell` installation is already
available in the system.

Only installations on another Linux host need to ensure that `bash`, `curl`,
`jq`, and `sha256sum` are available. Manually extracting the offline GitHub
Release bundle additionally requires `tar`.

The server running ClubFoundry must be able to reach the TrueNAS management API
and the iSCSI network it will manage.

## Option 1: self-contained GitHub Release bundle

This is the installation path that does not download application images from
another service.

1. Open [GitHub Releases](https://github.com/ClubFoundry/clubfoundry/releases).
2. Select the required release. Releases marked **Pre-release** are alpha
   builds and should be evaluated accordingly.
3. Download `clubfoundry-VERSION-offline.tar.gz` and `SHA256SUMS`.
4. Verify the outer archive against the matching line in `SHA256SUMS`.
5. Copy the archive to the server, extract it, and run its installer.

```bash
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf clubfoundry-VERSION-offline.tar.gz
cd clubfoundry-VERSION-offline
sudo bash install.sh --offline="$PWD"
```

The installer verifies its `MANIFEST`, installer script, and both image
archives against the bundle's internal `SHA256SUMS` before running
`docker load`.

For a non-TrueNAS Linux Docker host, select a persistent directory explicitly:

```bash
sudo bash install.sh --offline="$PWD" \
  --app-dir=/srv/clubfoundry
```

## Option 2: online installer from GitHub

This script is downloaded from GitHub, but the compiled images are resolved
through ClubFoundry's verified release service and mirrors.

```bash
curl -fL \
  https://raw.githubusercontent.com/ClubFoundry/clubfoundry/main/installer/install.sh \
  -o install.sh
less install.sh
sudo bash install.sh
```

For unattended installation, explicitly set the per-instance usage mode:

```bash
sudo bash install.sh --usage-mode=noncommercial
# or
sudo bash install.sh --usage-mode=commercial
```

A fresh interactive installation asks for this choice. Existing installations
retain their saved choice. A legacy installation without a saved choice is
classified once by the updated application and the result is persisted.

## TrueNAS persistent data

The installer prefers the pool selected for TrueNAS Apps. If there is no Apps
pool and exactly one suitable mounted ZFS data pool exists, it uses that pool.
If several pools are available, choose one explicitly:

```bash
sudo bash install.sh --app-dir=/mnt/POOL/clubfoundry
```

Do not place persistent application data in `/opt`, `/var/lib`, the TrueNAS boot
pool, or another non-persistent boot-environment path.

## First start

The installer prints the selected web address, normally
`http://SERVER-IP:3000`. Open it in a browser, accept the current EULA, and use
the setup screen to provide the TrueNAS host and API key. The API key is not
accepted as a public command-line argument and must not be committed to Git.

## Updates

The normal Compose installation includes the updater sidecar. Use the update
page in ClubFoundry. A manual online refresh is available with:

```bash
sudo bash install.sh --update
```

For an offline update, copy and extract the newer bundle, then run:

```bash
sudo bash NEW-BUNDLE/install.sh --update --offline=/absolute/path/to/NEW-BUNDLE
```

## Dry run and destructive reset

`--dry-run` prints the resolved plan without modifying files, images, or
containers:

```bash
sudo bash install.sh --dry-run --app-dir=/mnt/POOL/clubfoundry
```

`--clean-install` deletes the existing ClubFoundry database and state. It is not
an ordinary reinstall or update command. Use it only when that data is no
longer needed and a suitable backup exists.

## Windows Agent

Stop here if only server-side TrueNAS/ZFS/iSCSI management is required. The
optional Windows component has a separate [Windows Agent guide](WINDOWS_AGENT.md).
