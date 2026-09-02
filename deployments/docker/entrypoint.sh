#!/bin/sh
# Production entrypoint: ensure storage directories exist with the
# right ownership, then exec the backend as the unprivileged appuser.

set -eu

for path in "${APK_STORAGE_PATH:-/app/apk_storage}" "${IMAGE_STORAGE_PATH:-/app/image_storage}"; do
    mkdir -p "$path"
    chown appuser:appgroup "$path"
done

exec su-exec appuser:appgroup "$@"