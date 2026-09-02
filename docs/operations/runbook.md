# Operations Runbook

Day-to-day procedures for the on-call operator. Pair this with
[`README.md`](README.md) (entry point, env vars, checklist) and
[`rollback.md`](rollback.md) (recovery procedures).

## Alert response

| Alert | First action |
|---|---|
| `iptv_service_healthcheck_failed` | `sudo systemctl status iptv.service`, `sudo journalctl -u iptv.service -n 200 --no-pager` |
| `iptv_db_connections_exhausted` | Check `DB_MAX_OPEN_CONNS`; check for stuck transactions via `SHOW PROCESSLIST` |
| `iptv_redis_outage` | See [Redis Outage Behavior](#redis-outage-behavior) |
| `iptv_apk_storage_full` | `du -sh /opt/iptv/apk_storage/*`; prune old upload chunks |
| `iptv_scrape_403` | SofaScore is blocking this IP. Cosmetic; scraper retries on next tick. |
| `iptv_migration_dirty` | `SELECT * FROM iptv.schema_migrations` — should be `dirty=false`. If dirty, see [`rollback.md`](rollback.md). |

## Redis outage behavior

The backend uses Redis for two things: distributed locks for scheduled
jobs and the realtime WebSocket fanout. Both are designed to fail soft:

- **Distributed locks** — if Redis is down, scheduled jobs are skipped
  on this backend. Other backends still pick up the work. When Redis
  returns, locks resume normally.
- **Realtime WebSocket** — clients see a `WebSocketChannelException`
  and reconnect. The backend logs the disconnect; no data is lost.

When Redis recovers, no manual intervention is needed.

## Upload cleanup

Old APK upload chunks accumulate in `APK_STORAGE_PATH` if clients abort
uploads mid-way. The cleanup job runs every 15 minutes and removes
chunks from aborted uploads older than 1 hour. If the job itself fails,
the next tick retries; nothing else to do unless the disk is full.

## Counter recovery

APK download counters are flushed every 15 minutes. If the backend is
killed mid-flush, the `download_counter_flushes` table may contain a
`dirty=true` row. On the next start the counter is re-inserted
(`ReprocessOrphans`) and the flush resumes.

Manual check:

```bash
sudo mariadb -uroot iptv -e "SELECT * FROM download_counter_flushes ORDER BY id DESC LIMIT 5"
```

If a row is stuck, the next automatic flush will pick it up.

## Cache invalidation

There is no application cache to invalidate manually. Static assets
are served from the binary's embedded `web/dist`; the cache headers
are `Cache-Control: public, max-age=...` based on file hashes, so
clients pick up new builds automatically.

For dashboard layout issues that survive a hard refresh, force-reload
the page (`Ctrl+Shift+R` / `Cmd+Shift+R`).

## Common operational commands

```bash
# Service
sudo systemctl status iptv.service
sudo systemctl restart iptv.service
sudo journalctl -u iptv.service -f
sudo journalctl -u iptv.service -n 200 --no-pager

# Health
curl -s http://127.0.0.1:8080/health/live
curl -s http://127.0.0.1:8080/health/ready

# Logs
sudo tail -f /var/log/iptv.log           # if not using journald
sudo journalctl -u iptv.service --since "1 hour ago"

# Database
sudo mariadb -uroot iptv -e "SHOW TABLES"
sudo mariadb -uroot iptv -e "SELECT * FROM schema_migrations"
sudo mariadb -uroot iptv -e "SHOW PROCESSLIST"

# Storage
du -sh /opt/iptv/apk_storage /opt/iptv/image_storage
ls -lah /opt/iptv

# Redis
redis-cli ping
redis-cli INFO clients
```

## First Boot

On a fresh database (where `users` is empty), the server auto-seeds a default administrator:

- **Email:** `admin@local`
- **Password:** `admin1234`
- **Role:** `admin`

> **WARNING:** The default password is hardcoded and known. **Change it on first login.** Operators who want a different password must `DELETE FROM users WHERE email = 'admin@local'` after the first boot and re-invoke the bootstrap flow.

To pre-apply schema and exit (no server), run:

```bash
./sofascore-scrapper migrate
```

This runs `AutoMigrateAll` and the seeder, then exits 0.

To skip migration and seeding entirely (when managing schema externally):

```bash
SKIP_MIGRATE=true ./sofascore-scrapper
```
