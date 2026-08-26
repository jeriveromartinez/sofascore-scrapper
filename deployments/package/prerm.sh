#!/bin/sh
set -e

if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    systemctl stop iptv.service >/dev/null 2>&1 || true
    systemctl disable iptv.service >/dev/null 2>&1 || true
fi

exit 0
