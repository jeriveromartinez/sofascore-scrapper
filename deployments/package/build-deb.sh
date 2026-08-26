#!/usr/bin/env bash
set -Eeuo pipefail

# Builds the .deb package via Docker.
#
# Usage:
#   ./deployments/package/build-deb.sh [version]
#
#   version  defaults to the latest git tag (fallback 0.1.0)
# Output lands in dist/iptv_<version>_amd64.deb

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/../.." && pwd)
version=${1:-$(git -C "$repo_root" describe --tags --abbrev=0 2>/dev/null || echo 0.1.0)}

image="iptv-deb-builder:${version}"
out_dir="$repo_root/dist"
mkdir -p "$out_dir"

echo ">> Building $image with version $version"

docker build \
    -f "$repo_root/deployments/package/Dockerfile.builder" \
    --build-arg "DEB_VERSION=$version" \
    -t "$image" \
    "$repo_root"

echo ">> Extracting .deb to $out_dir"

# Use docker create + docker cp to avoid MSYS path mangling of /bin/sh on
# Windows/Git Bash hosts.
container=$(docker create "$image")
trap 'docker rm "$container" >/dev/null 2>&1 || true' EXIT
docker cp "$container:/iptv.deb" "$out_dir/iptv_${version}_amd64.deb"
docker rm "$container" >/dev/null 2>&1 || true
trap - EXIT

echo ">> Done: $out_dir/iptv_${version}_amd64.deb"
