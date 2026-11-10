# syntax=docker/dockerfile:1

# The omega command in a Debian container, as ghcr.io/lcajigasm/omega.
# A release builds it for linux/amd64 and linux/arm64. To build it here:
#
#   docker build --build-arg VERSION=$(git describe --tags --always) -t omega .

# The builder runs on the machine's own architecture and cross-compiles, so
# an arm64 image does not compile under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/omega ./cmd/omega

# omega runs shell commands, so the image is a small system and not an empty
# one: bash and apt come with Debian. ca-certificates is for the providers'
# HTTPS, and sudo because the apt steps call it (root needs no password).
FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates sudo \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/omega /usr/local/bin/omega
COPY LICENSE /usr/share/doc/omega/LICENSE
# No terminal to draw on unless the container is started with -it.
ENV TERM=xterm-256color
ENTRYPOINT ["omega"]
