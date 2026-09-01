#!/bin/bash
# Chain-test driver: install every iptv .deb in /tmp/dist in order
# (oldest filename first), start the iptv binary, wait for /health/live,
# then stop the binary and move on to the next version.
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

# ---------------------------------------------------------------------------
# Smoke test: verify migration 000017 was applied to the live DB and that
# the new deleted_at column accepts writes on the three push tables.
# This guards against the same class of bug as PR #101 (Error 1054
# "Unknown column 'deleted_at' in 'INSERT INTO'" on push_messages,
# scheduled_pushes, delivery_attempts) regressing.
# ---------------------------------------------------------------------------
log "Smoke test: verify deleted_at on push_messages / scheduled_pushes / delivery_attempts"

# 1. Columns + indexes must exist (migration 000017 effect).
for table in push_messages scheduled_pushes delivery_attempts; do
    has_col=$(mariadb -uroot -BN -e \
        "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='iptv' AND TABLE_NAME='$table' AND COLUMN_NAME='deleted_at'")
    if [ "$has_col" -ne 1 ]; then
        log "  FAIL: $table.deleted_at column missing"
        exit 1
    fi
    log "  OK  $table.deleted_at column present"
done
for idx in idx_push_messages_deleted_at idx_scheduled_pushes_deleted_at idx_delivery_attempts_deleted_at; do
    has_idx=$(mariadb -uroot -BN -e \
        "SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='iptv' AND INDEX_NAME='$idx'")
    if [ "$has_idx" -lt 1 ]; then
        log "  FAIL: index $idx missing"
        exit 1
    fi
    log "  OK  index $idx present"
done

# 2. INSERT smoke: the columns must accept NULL (soft-delete = no row
# yet) and a real timestamp. Foreign keys are disabled because the
# chain test does not bootstrap a user/device/tournament.
mariadb -uroot iptv >/dev/null 2>&1 <<'SQL' || { log "  FAIL: smoke INSERT into push_messages failed"; exit 1; }
SET FOREIGN_KEY_CHECKS=0;
INSERT INTO push_messages (created_at, updated_at, deleted_at, user_id, category, title, body, image_url, deep_link, priority, ttl_seconds, data_json, source, scheduled_id)
  VALUES (NOW(3), NOW(3), NULL, 0, 'admin_message', 'chain_smoke', 'chain_smoke', '', '', 'normal', 0, NULL, 'immediate', NULL);
SET FOREIGN_KEY_CHECKS=1;
SQL
log "  OK  push_messages INSERT (deleted_at IS NULL)"

mariadb -uroot iptv >/dev/null 2>&1 <<'SQL' || { log "  FAIL: smoke INSERT into scheduled_pushes failed"; exit 1; }
SET FOREIGN_KEY_CHECKS=0;
INSERT INTO scheduled_pushes (created_at, updated_at, deleted_at, user_id, schedule_type, next_fire_at, category, title, body, priority, ttl_seconds)
  VALUES (NOW(3), NOW(3), NULL, 0, 'one_shot', NOW(3), 'admin_message', 'chain_smoke', 'chain_smoke', 'normal', 0);
SET FOREIGN_KEY_CHECKS=1;
SQL
log "  OK  scheduled_pushes INSERT (deleted_at IS NULL)"

mariadb -uroot iptv >/dev/null 2>&1 <<'SQL' || { log "  FAIL: smoke INSERT into delivery_attempts failed"; exit 1; }
SET FOREIGN_KEY_CHECKS=0;
INSERT INTO delivery_attempts (created_at, updated_at, deleted_at, push_message_id, device_id, message_id, state)
  VALUES (NOW(3), NOW(3), NULL, 0, 0, '00000000-0000-0000-0000-000000000000', 'sent');
SET FOREIGN_KEY_CHECKS=1;
SQL
log "  OK  delivery_attempts INSERT (deleted_at IS NULL)"

# 3. Clean up the smoke rows so the user starts with a clean slate.
mariadb -uroot iptv >/dev/null 2>&1 <<'SQL'
SET FOREIGN_KEY_CHECKS=0;
DELETE FROM delivery_attempts  WHERE message_id = '00000000-0000-0000-0000-000000000000';
DELETE FROM scheduled_pushes  WHERE title      = 'chain_smoke';
DELETE FROM push_messages     WHERE title      = 'chain_smoke';
SET FOREIGN_KEY_CHECKS=1;
SQL
log "  OK  smoke rows cleaned up"

# Keep the final version running so the user can curl health/live.
load_env

# Smoke-test the crash-report endpoint with a synthetic payload. The
# chain bug we are validating reproduces here as `Error 1054:
# Unknown column 'name' in 'INSERT INTO'` because the GORM model
# `reporting.CrashReport` writes embedded `AppReport` / `DeviceReport`
# fields as bare column names (no `embeddedPrefix`) and migration
# 000001 only created the prefixed `app_*` / `device_*` columns.
# Migration 000016 adds the bare columns non-destructively.
: > /tmp/iptv.log
sudo -u iptv -E /opt/iptv/iptv >/tmp/iptv.log 2>&1 &
final_pid=$!
for _ in $(seq 1 60); do
    if wget -qO- http://localhost:8080/health/live >/dev/null 2>&1; then break; fi
    if ! kill -0 "$final_pid" 2>/dev/null; then
        log "  FAIL: iptv exited before becoming healthy"
        tail -n 30 /tmp/iptv.log | sed 's/^/    /'
        exit 1
    fi
    sleep 1
done
wget -qO- http://localhost:8080/health/live >/dev/null 2>&1 || {
    log "  FAIL: /health/live did not return 200 after restart"
    tail -n 30 /tmp/iptv.log | sed 's/^/    /'
    kill "$final_pid" 2>/dev/null || true
    exit 1
}

crash_payload='{"fatal":true,"error":"synthetic chain harness","stackTrace":"","context":"chain test","app":{"name":"ChainHarness","version":"0.0.0","build":"0","environment":"test","platform":"linux"},"device":{"osVersion":"Ubuntu 24.04","locale":"en_US"}}'
http_code=$(wget -qSO- \
    --header="Content-Type: application/json" \
    --post-data="$crash_payload" \
    http://localhost:8080/api/app/v1/crash-report 2>&1 \
    | grep -m1 'HTTP/' \
    | head -n1 \
    | awk '{print $2}')
log "  POST /api/app/v1/crash-report -> HTTP $http_code"
if [ "$http_code" != "200" ]; then
    log "  FAIL: crash-report endpoint returned $http_code, want 200 (schema mismatch regression?)"
    log "  --- last 30 lines of iptv.log ---"
    tail -n 30 /tmp/iptv.log | sed 's/^/    /'
    kill "$final_pid" 2>/dev/null || true
    exit 1
fi

# Verify the row landed in crash_reports with the values we just sent.
# If the table is missing columns, GORM's INSERT would have failed
# above; this double-check protects against silent regressions where
# the handler returns 200 but the row never persisted.
if command -v mariadb >/dev/null 2>&1; then
    if ! mariadb -uroot -e "SELECT name, version, environment, platform, locale FROM iptv.crash_reports ORDER BY id DESC LIMIT 1" 2>/dev/null \
        | grep -q 'ChainHarness.*0\.0\.0.*test.*linux.*en_US'; then
        log "  FAIL: crash_reports row not found with expected payload"
        log "  --- crash_reports contents ---"
        mariadb -uroot -e "SELECT id, name, version, environment, platform, locale FROM iptv.crash_reports" 2>/dev/null | sed 's/^/    /'
        kill "$final_pid" 2>/dev/null || true
        exit 1
    fi
    log "  crash_reports row verified (name=ChainHarness, version=0.0.0, environment=test, platform=linux, locale=en_US)"
fi

# Stop the verification instance so the exec below can bind port 8080
# cleanly for the user.
log "  stopping verification instance (pid $final_pid)"
kill "$final_pid" 2>/dev/null || true
wait "$final_pid" 2>/dev/null || true
sleep 2

exec sudo -u iptv -E /opt/iptv/iptv
