# Sidecar release evidence

[Русская версия](RELEASE_EVIDENCE.ru.md)

A `sidecar-v*` tag is the only workflow event that publishes the updater image.
Branch and pull-request runs remain build-only checks.

For an authorized tag, `.github/workflows/sidecar-release.yml` publishes one
multi-architecture image index and records:

- the immutable OCI manifest digest;
- a BuildKit SPDX SBOM attestation;
- max-level BuildKit provenance;
- a GitHub-signed image attestation bound to the immutable digest;
- an Actions evidence artifact containing the digest, exported SBOM and
  provenance JSON, notices, license texts, corresponding-source offer, and
  SHA-256 checksums for the exported files.

The workflow must fail if the generated runtime package inventory differs from
`runtime-packages.lock`, a digest-pinned Docker tool differs from
`runtime-tools.lock`, a required license text is absent or modified, the license
directory is missing from the image, the image has a fixable Critical
vulnerability, or either attached attestation cannot be exported after
publication.

## Local checks

```bash
python sidecar/verify-release-evidence.py
docker build --build-arg VERSION=evidence-check \
  -t clubfoundry-updater:evidence-check sidecar
bash sidecar/audit-runtime-licenses.sh clubfoundry-updater:evidence-check \
  | diff -u sidecar/runtime-packages.lock -
docker run --rm --entrypoint docker clubfoundry-updater:evidence-check \
  --version
docker run --rm --entrypoint docker clubfoundry-updater:evidence-check \
  compose version --short
```

## Published-image checks

Replace the example digest with the value from the release evidence:

```bash
image=ghcr.io/clubfoundry/updater@sha256:EXACT_DIGEST
docker buildx imagetools inspect "$image" --format '{{json .SBOM}}'
docker buildx imagetools inspect "$image" --format '{{json .Provenance}}'
gh attestation verify "oci://$image" --repo ClubFoundry/clubfoundry
```

The image tag, `/health` version, and self-update sentinel must also report the
same version. That runtime identity check is performed on the first authorized
tag and is not replaced by supply-chain attestations.
