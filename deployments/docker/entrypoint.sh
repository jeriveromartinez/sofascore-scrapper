#!/bin/sh
# Production entrypoint: ensure storage directories exist with the
# right ownership, then exec the backend as the unprivileged appuser.

set -eu

for path in "${APK_STORAGE_PATH:-/app/apk_storage}" "${IMAGE_STORAGE_PATH:-/app/image_storage}"; do
    mkdir -p "$path"
    chown appuser:appgroup "$path"
done

# rod downloads its managed Chromium to $HOME/.cache/rod on first
# launch. The compose dev stack mounts a volume there; the mount
# can land owned by root, which would block the unprivileged
# appuser from writing the binary. Re-chown just in case.
if [ -d /home/appuser/.cache ]; then
    chown -R appuser:appgroup /home/appuser || true
fi

exec su-exec appuser:appgroup "$@"