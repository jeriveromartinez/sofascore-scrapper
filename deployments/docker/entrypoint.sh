#!/bin/sh
set -eu

for path in "${APK_STORAGE_PATH:-/app/apk_storage}" "${IMAGE_STORAGE_PATH:-/app/image_storage}"; do
	mkdir -p "$path"
	chown appuser:appgroup "$path"
done

exec su-exec appuser:appgroup "$@"
