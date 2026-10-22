#!/bin/sh
# Post-removal: drop the systemd unit and the application directory so the
# package can be uninstalled without leaving anything behind.
#
# dpkg calls this script with:
#   remove       the package is being removed (configuration kept)
#   purge        the package and its configuration are being removed
#   upgrade      an older version is being replaced (keep everything)
set -e

case "$1" in
    remove|purge)
        systemctl stop iptv.service >/dev/null 2>&1 || true
        systemctl disable iptv.service >/dev/null 2>&1 || true

        rm -f /lib/systemd/system/iptv.service /etc/systemd/system/iptv.service
        systemctl daemon-reload >/dev/null 2>&1 || true
        systemctl reset-failed iptv.service >/dev/null 2>&1 || true

        rm -rf /opt/iptv

        if [ "$1" = "purge" ]; then
            rm -f /etc/iptv/env
            rmdir /etc/iptv >/dev/null 2>&1 || true
        fi
        ;;
esac

exit 0
