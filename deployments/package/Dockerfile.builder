# Builder image: compiles the Go backend, builds the Vue dashboard, and
# assembles a .deb package for Ubuntu (systemd service included).
#
# Build:
#   docker build -f deployments/package/Dockerfile.builder -t iptv-deb-builder .
#   docker run --rm -v "$PWD/dist:/out" iptv-deb-builder
# Output: dist/iptv_<version>_amd64.deb

FROM node:22-alpine AS build-vue

WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build-go

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION="v0.0.0-dev"
ARG COMMIT="unknown"
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags="-s -w -X github.com/jeriveromartinez/sofascore-scrapper/internal/buildinfo.Version=${VERSION} -X github.com/jeriveromartinez/sofascore-scrapper/internal/buildinfo.Commit=${COMMIT}" \
    -o /out/sofascore-scrapper ./cmd/server

FROM ubuntu:22.04 AS package

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends dpkg-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /pkg

# Package metadata and maintainer scripts
COPY deployments/package/control ./DEBIAN/control
COPY deployments/package/postinst.sh ./DEBIAN/postinst
COPY deployments/package/prerm.sh ./DEBIAN/prerm
COPY deployments/package/iptv.service ./lib/systemd/system/iptv.service

# Binaries and dashboard
COPY --from=build-go /out/sofascore-scrapper ./opt/iptv/iptv
COPY --from=build-vue /app/web/dist ./opt/iptv/web/dist

RUN chmod 0755 ./DEBIAN/postinst ./DEBIAN/prerm \
    && chmod 0755 ./opt/iptv/iptv

# Version comes from a build arg so the .deb version matches the repo tag.
ARG DEB_VERSION=0.1.0
RUN sed -i "s/^Version: .*/Version: ${DEB_VERSION}/" ./DEBIAN/control

RUN mkdir -p /out \
    && dpkg-deb --build --root-owner-group . /out/iptv_${DEB_VERSION}_amd64.deb \
    && cp /out/iptv_${DEB_VERSION}_amd64.deb /iptv.deb

# The image doubles as an artifact container: docker create + docker cp pull
# the .deb out without needing a shell.
CMD ["/bin/true"]
