# ─── Build stage ───────────────────────────────────────────────────────────
FROM golang:1.26.5-alpine AS build

WORKDIR /src

# Copy the module definition first so dependency resolution is cached
# independently from source changes. This module currently has no go.sum.
COPY go.mod ./
RUN go mod download

# Copy source and build a statically-linked binary for the target arch.
# ARG TARGETARCH is automatically populated by `docker buildx` multi-arch
# builds — no-op for single-arch local builds where it defaults to host arch.
COPY . ./
ARG TARGETOS=linux
ARG TARGETARCH
# VERSION has no default because self-update verifies the compiled version
# against the requested target. Rejecting "dev" prevents an unrecoverable
# version mismatch after the container is recreated.
ARG VERSION
RUN test -n "${VERSION}" -a "${VERSION}" != "dev" \
    || (echo "ERROR: docker build must pass --build-arg VERSION=<tag> (got '${VERSION}')." \
        && echo "       Sidecar self-update sentinel will reject any binary reporting version='dev'." \
        && exit 1)

# CGO_ENABLED=0 guarantees a static binary that can run anywhere.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/updater \
      ./cmd/updater

# Use Docker's digest-pinned Compose binary so the plugin version is reviewed
# independently from Alpine package updates for each target architecture.
FROM docker/compose-bin:v5.4.0@sha256:b03e46987ca4ebb41ca31b765ad7ba957388003f1ef1255fd68333a7b838d632 AS compose

# Use Docker's official static CLI binary so its Go toolchain is reviewed
# independently from Alpine's package rebuild cadence.
FROM docker:29.5.3-cli-alpine3.24@sha256:11e1133c30f3ceb73c6bdc7dfb78b3f9ed8e8e0d1d0400e91c5ec2eb240bf2ff AS docker_cli

# ─── Runtime stage ─────────────────────────────────────────────────────────
# Alpine with Docker CLI + Compose plugin so the sidecar can shell out to
# `docker compose pull/up/ps`. The Docker socket at /var/run/docker.sock is
# mounted read/write (NOT read-only — compose needs to start/stop containers)
# by docker-compose.yml. The sidecar never runs a Docker DAEMON — it only
# talks to the host's daemon via the mounted socket.
#
# FROM scratch would be ~10 MB but can't shell out to docker. Alpine +
# Docker CLI + Compose is ~30 MB — acceptable for a rarely-run updater.
FROM alpine:3.24

RUN apk add --no-cache \
      ca-certificates \
      tzdata \
 && rm -rf /var/cache/apk/*

COPY --from=docker_cli /usr/local/bin/docker /usr/bin/docker
COPY --from=compose /docker-compose /usr/libexec/docker/cli-plugins/docker-compose
COPY --from=build /out/updater /updater
COPY THIRD_PARTY_NOTICES.md /THIRD_PARTY_NOTICES.md
COPY SOURCE_OFFER.md /SOURCE_OFFER.md
COPY LICENSES /licenses

EXPOSE 3001
ENTRYPOINT ["/updater"]
