# ClubFoundry

[Русская версия](README.ru.md)

ClubFoundry automates game-library updates and distribution for gaming clubs
using TrueNAS. Windows boots from each PC's local system disk,
while the game libraries reside on a separate network-accessible ZVOL clone of
the master disk for each computer and are attached through the built-in Windows
iSCSI Initiator. This hybrid architecture is known as `half-diskless`.

This removes the need to store the entire game library on every computer's
local SSD and significantly reduces the required capacity of those SSDs.

ZFS clones do not duplicate the entire master disk: unchanged data blocks
remain shared, and additional server storage is consumed only by data that
differs between the master disk, retained versions, and individual clones.
Thirty clones of a 10 TB library therefore do not themselves require 300 TB of
physical storage. The baseline calculation is the 10 TB source library plus
10-200 GB of change allowance for each club PC. Retaining more versions or
allowing more individual changes increases actual space consumption.

ClubFoundry can manage multiple source game ZVOLs, also called
`source/master disks`, `golden disks`, or `source ZVOLs`. For each PC, the
application creates a separate writable clone of the selected master disk and
manages its assignment to that computer.

After games are updated on the master disk, ClubFoundry creates a new version
and can centrally move selected computers or the entire club to it, effectively
rolling out the update. Each PC uses its own clone, so changes made by one
computer do not affect the master disk or any other computer.

## Install

The simplest option is to connect to TrueNAS over SSH and run the online
installer with one command:

```bash
ssh ADMIN@TRUENAS_IP
curl -fsSL https://raw.githubusercontent.com/ClubFoundry/clubfoundry/main/installer/install.sh | sudo bash
```

For step-by-step TrueNAS preparation and an explanation of each action, use the
[simple website installation guide](https://clubfoundry.net/install).

The GitHub release also contains a ready-to-install offline bundle with the
compiled ClubFoundry application and updater images for TrueNAS SCALE and other
Linux Docker hosts. This is the manual option for technical operators who want
to download and verify the release files themselves.

See the [detailed repository installation guide](INSTALLATION.md) for
requirements, commands, usage-mode selection, updates, and removal warnings.

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
