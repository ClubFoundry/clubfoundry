# ClubFoundry

[Русская версия](README.ru.md)

ClubFoundry provides centralized per-PC iSCSI game-disk management for
TrueNAS SCALE. It creates separate writable ZVOL snapshot clones for managed
PCs while Windows continues to boot from each PC's local system disk.
ClubFoundry is not a diskless boot, PXE, iPXE, or iSCSI boot system.

## Install

The GitHub release contains a ready-to-install offline bundle with the compiled
ClubFoundry application and updater images. It can be installed on TrueNAS
SCALE or on another Linux Docker host. An online installer is also available
from this repository.

See [Installation](INSTALLATION.md) for requirements, commands, usage-mode
selection, updates, and removal warnings.

## Optional Windows Agent

The Windows Agent is not required for the core TrueNAS, ZFS, ZVOL, and iSCSI
management workflow. Install it only on Windows PCs where Windows-side
automation, live status, remote commands, or Steam automation is needed.

See [Windows Agent](WINDOWS_AGENT.md). Do not install the MSI on TrueNAS.

## Repository contents

This public repository intentionally contains only:

- source code for the installer;
- source code for the updater sidecar;
- deployment and user documentation;
- compiled application images and the compiled Windows Agent as GitHub Release
  assets, not as files in Git history.

The main ClubFoundry application and Windows Agent source code are proprietary
and are not published here. See [Licensing](LICENSE.md) for the exact boundary.

## Usage modes

Each installation selects its own mode:

- Personal non-commercial use is free for up to 30 managed non-admin PCs.
- Commercial or professional use includes the first 10 managed non-admin PCs
  free of charge, with up to 5 ZVOLs per managed PC. Additional managed PCs and
  paid add-ons are billed under the current terms shown by the application.

One account may contain both commercial and non-commercial installations.
Current terms are available at <https://clubfoundry.net/eula> and
<https://clubfoundry.net/pricing>.

## Support and security

For support, use <https://t.me/ClubFoundryBot> or
`support@clubfoundry.net`. Do not publish API keys, account-link tokens,
Windows Agent boot tokens, `.env` files, databases, or diagnostic archives in
GitHub issues.
