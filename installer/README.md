# ClubFoundry — Installer

One-command installer for TrueNAS SCALE. Verifies Docker, bootstraps the
images, creates the data directory, and starts ClubFoundry — the app container
plus the `clubfoundry-updater` sidecar (auto-updates, safe rollback,
crash-loop protection).

`installer/install.sh` is the **canonical source** of the install script.
`site/install.sh` is a published copy kept in sync automatically by
`cloud/scripts/deploy-clubfoundry-site.mjs` — **edit `installer/install.sh` only.**

## Usage

```bash
curl -fsSL https://clubfoundry.net/install.sh | bash
```

The installer is **fully non-interactive** — it asks no questions:

- **TrueNAS API key + host** are NOT collected in the terminal. The app's
  first-run browser screen collects and validates them (copy-paste, 14
  languages). Power users / CI can pre-seed via env to skip that gate:
  `CLM_TRUENAS_API_KEY=… CLM_TRUENAS_HOST=http://localhost bash install.sh`.
- **Containers.** The installer always sets up the app + `clubfoundry-updater`
  sidecar as a Docker Compose stack. A single-container install with no sidecar
  (and therefore no auto-update) is only a power-user fallback — chosen
  automatically on the rare host without `docker compose`, or forced with
  `--mode=a`.
- **Web UI port** defaults to 3000, auto-falling back to a free port if taken.
  Override with `--port=N`.

Other flags: `--channel=stable|beta|lts`, `--link-token=cflink_…` (the Account
Portal one-liner), `--offline=<dir>` (air-gapped bundle), `--app-dir=<path>`,
`--update`, `--dry-run`, and `--help`. Unknown flags fail before any host changes.
`--dry-run` prints the resolved plan without writing files, stopping containers,
or deleting data.

`--clean-install` is deliberately destructive: it removes the existing
ClubFoundry database and state after confirming that any same-named containers
belong to ClubFoundry. Use it only when existing data is no longer needed.

## What It Does

1. Pre-flight: root check, TrueNAS-version-aware Docker availability checks.
2. Resolves config (mode, port, app dir, channel) — no prompts.
3. Bootstraps images: integrity-verified tarballs from the cloud (or a local
   bundle with `--offline`).
4. Writes `.env` + `docker-compose.yml`, starts the container(s). If replacement
   Compose startup fails during a rerun, it restores the previous configuration
   and starts the previous stack again.
5. Observes the startup health checks, then prints the Web UI URL and next steps.
   A timeout is reported as a warning because a slow first boot may still finish;
   it does not trigger an installer rollback.

## Update

```bash
bash install.sh --update
```

Reuses the existing port and config. A normal install updates itself via the
sidecar; `bash install.sh --update` is the manual path for a single-container
`--mode=a` install.

## Testing

Fast checks do not touch Docker or TrueNAS:

```bash
bash -n installer/install.sh installer/test-fresh-install-e2e.sh
sh -n installer/bootstrap.sh
bash installer/install.sh --help
bash installer/test-installer-contracts.sh
```

`test-fresh-install-e2e.sh` runs the privileged fresh-install scenario against
an explicitly configured disposable TrueNAS test system. Review its header and
environment variables before invocation; never point it at a customer system.

## Published To

All hosted copies are re-synced automatically by
`cloud/scripts/deploy-clubfoundry-site.mjs` on every site deploy:

- Cloudflare Pages — `clubfoundry.net/install.sh` (primary)
- Selectel releases bucket — `…selstorage.ru/install.sh` (CIS fallback)
- `mirror.clubfoundry.ru/install.sh` (CIS fallback)
- GitHub `ClubFoundry/clubfoundry` (public repo)

## License

MIT; see [`LICENSE.md`](LICENSE.md). Users can inspect what runs on their server.
