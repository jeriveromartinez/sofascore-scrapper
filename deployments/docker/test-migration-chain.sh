#!/bin/bash
# Test driver: install every iptv .deb in /tmp/dist in order (oldest
# filename first), start the iptv binary, wait for /health/live, then
# stop the binary and move on to the next version.
#
# Versions that fail to start (e.g. 0.0.5 shipped with a duplicate
# migration file that was fixed in 0.0.6) are logged and skipped. The
# test only fails if the LAST .deb in the chain — the one containing
# the current proposal — does not come up healthy.
set -uo pipefail

log() { echo "[$(date -u +%H:%M:%SZ)] $*"; }

# Start MariaDB and Redis. The .deb's postinst expects both to be up
# and creates the iptv database + user with a random password that
# ends up in /etc/iptv/env. We MUST NOT pre-create the user ourselves
# (CREATE USER IF NOT EXISTS does not update the password, so the env
# file and the actual grant would diverge).
log "Starting MariaDB..."
mkdir -p /var/run/mysqld
chown -R mysql:mysql /var/run/mysqld
service mariadb start
for _ in $(seq 1 30); do
    if mariadb-admin ping --silent >/dev/null 2>&1; then break; fi
    sleep 1
done
mariadb-admin ping --silent >/dev/null 2>&1 || { log "FAIL: MariaDB did not start"; exit 1; }

# Drop any leftover iptv database/user from a previous container run.
mariadb -uroot <<'SQL' 2>/dev/null || true
DROP DATABASE IF EXISTS iptv;
DROP USER IF EXISTS 'iptv'@'localhost';
DROP USER IF EXISTS 'iptv'@'127.0.0.1';
SQL

log "Starting Redis..."
service redis-server start
for _ in $(seq 1 15); do
    if redis-cli ping >/dev/null 2>&1; then break; fi
    sleep 1
done
redis-cli ping >/dev/null 2>&1 || { log "FAIL: Redis did not start"; exit 1; }

load_env() {
    if [ ! -f /etc/iptv/env ]; then
        return 1
    fi
    set -a
    # shellcheck disable=SC1091
    . /etc/iptv/env
    set +a
}

# Try a single .deb. Returns 0 if the service became healthy, 1
# otherwise. Never aborts the script.
try_one() {
    local deb="$1"
    log "===== Installing $deb ====="
    if ! DEBIAN_FRONTEND=noninteractive dpkg -i "$deb" 2>&1 | tail -n 3; then
        log "  dpkg install failed for $deb"
        return 1
    fi

    load_env || { log "  no /etc/iptv/env after install"; return 1; }

    : > /tmp/iptv.log
    sudo -u iptv -E /opt/iptv/iptv >/tmp/iptv.log 2>&1 &
    local pid=$!

    for _ in $(seq 1 60); do
        if wget -qO- http://localhost:8080/health/live >/dev/null 2>&1; then
            log "  healthy after $deb"
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
            sleep 1
            return 0
        fi
        if ! kill -0 "$pid" 2>/dev/null; then
            log "  iptv process exited prematurely (pid $pid)"
            break
        fi
        sleep 1
    done

    log "  FAIL: $deb did not become healthy"
    log "  --- last 15 lines of iptv.log ---"
    tail -n 15 /tmp/iptv.log | sed 's/^/    /'
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    sleep 1
    return 1
}

shopt -s nullglob
debs=( /tmp/dist/iptv_*.deb )
shopt -u nullglob

if [ ${#debs[@]} -eq 0 ]; then
    log "FAIL: no .deb files found in /tmp/dist"
    exit 1
fi

log "Versions to install (in order):"
for d in "${debs[@]}"; do log "  - $d"; done

declare -A results
for deb in "${debs[@]}"; do
    if try_one "$deb"; then
        results["$deb"]="OK"
    else
        results["$deb"]="FAIL"
    fi
done

log ""
log "===== Chain summary ====="
last_deb="${debs[${#debs[@]}-1]}"
last_status="FAIL"
for deb in "${debs[@]}"; do
    log "  $deb -> ${results[$deb]}"
done
last_status="${results[$last_deb]}"

if [ "$last_status" != "OK" ]; then
    log "FAIL: final version $last_deb did not start cleanly"
    exit 1
fi

log "===== Final version $last_deb is healthy; starting it for the user ====="

# Keep the final version running so the user can curl health/live.
load_env
: > /tmp/iptv.log
exec sudo -u iptv -E /opt/iptv/iptv
