#!/usr/bin/env bash
# Builds the .deb package via Docker. No host Go/Node toolchain
# required — the build runs entirely inside iptv-deb-builder.
#
# Usage:
#   ./deployments/package/build-deb.sh [-h|--help] [version]
#
# Output: dist/iptv_<version>_amd64.deb

set -Eeuo pipefail

usage() {
    sed -n '2,7p' "$0"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/../.." && pwd)

resolve_version() {
    local explicit="${1:-}"
    if [[ -n "$explicit" ]]; then
        printf '%s\n' "$explicit"
        return
    fi
    local tag
    tag=$(git -C "$repo_root" describe --tags --abbrev=0 2>/dev/null || true)
    if [[ -n "$tag" ]]; then
        printf '%s\n' "${tag#v}"
        return
    fi
    printf '%s\n' "0.1.0"
}

version=$(resolve_version "${1:-}")

if ! [[ "$version" =~ ^[0-9][0-9a-zA-Z\.+~-]*$ ]]; then
    echo ">> Invalid version '$version': must start with a digit and contain only [0-9a-zA-Z.+~-]" >&2
    exit 1
fi

image="iptv-deb-builder:${version}"
out_dir="$repo_root/dist"
mkdir -p "$out_dir"

echo ">> Building $image with version $version"

# Embed the git commit short-hash so the web UI can show
# "Build / Version: vX.Y.Z / Commit: <hash>".
commit=$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || echo "unknown")

docker build \
    -f "$repo_root/deployments/package/Dockerfile.builder" \
    --build-arg "DEB_VERSION=$version" \
    --build-arg "VERSION=v$version" \
    --build-arg "COMMIT=$commit" \
    -t "$image" \
    "$repo_root"

echo ">> Extracting .deb to $out_dir"

# docker create + docker cp avoids MSYS path mangling of /bin/sh on
# Windows / Git Bash hosts.
container=$(docker create "$image")
trap 'docker rm "$container" >/dev/null 2>&1 || true' EXIT
docker cp "$container:/iptv.deb" "$out_dir/iptv_${version}_amd64.deb"
docker rm "$container" >/dev/null 2>&1 || true
trap - EXIT

echo ">> Done: $out_dir/iptv_${version}_amd64.deb"
