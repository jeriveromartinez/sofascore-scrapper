#!/usr/bin/env bash
set -Eeuo pipefail

# Builds the .deb package via Docker.
#
# Usage:
#   ./deployments/package/build-deb.sh [-h|--help] [version]
#
#   version  defaults to the latest git tag with a leading 'v' stripped
#             (fallback 0.1.0 when no tag exists).
# Output lands in dist/iptv_<version>_amd64.deb

usage() {
    sed -n '2,11p' "$0"
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
    usage
    exit 0
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/../.." && pwd)

resolve_version() {
    local explicit="${1:-}"
    if [ -n "$explicit" ]; then
        printf '%s\n' "$explicit"
        return
    fi
    local tag
    tag=$(git -C "$repo_root" describe --tags --abbrev=0 2>/dev/null || true)
    if [ -n "$tag" ]; then
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

# Resolve the git commit short-hash to embed in the binary so the
# web UI can show "Build / Version: vX.Y.Z / Commit: <hash>".
commit=$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || echo "unknown")

docker build \
    -f "$repo_root/deployments/package/Dockerfile.builder" \
    --build-arg "DEB_VERSION=$version" \
    --build-arg "VERSION=v$version" \
    --build-arg "COMMIT=$commit" \
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
