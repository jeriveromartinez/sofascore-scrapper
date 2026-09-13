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
| `iptv_scraper_silent` | Logs show no `scraper: … errors` and `teams`/`events` tables are growing. If the table is stuck at zero rows, see [Scraper catalog](#scraper-catalog) — the FotMob endpoint URL changed in 2026; old builds hit `/api/leagues` (404). Rebuild from a branch with the b8d3291 fix (or later) and confirm `curl https://www.fotmob.com/api/data/matches?date=YYYYMMDD&timezone=Europe/Paris` returns 200. |

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
sudo mariadb -uroot iptv -e "SHOW CREATE TABLE users\\G"
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

## Bootstrap via invitation

If you prefer that the first operator register through the normal
invitation flow rather than logging in as `admin@local`, run:

```bash
./sofascore-scrapper bootstrap-invitation
```

The command prints the invitation token (also visible in the logs)
and refuses to run once `users` already has a row, so it must run
before the server's first normal boot. Start the server afterward
and the first human registers at `/register` using the token; that
account becomes the sole admin via the existing first-user rule.

## Scraper catalog

The scraper only runs against leagues present in the `scraper_leagues`
table with `enabled = true`. On a fresh DB the server auto-seeds a
curated list of leagues whose `source_league_id` was verified against
FotMob's real catalog on 2026-09-13. As of that date the seed has
41 entries (top 5 European leagues + second divisions + rest of
Europe top flights, the main LATAM / USA / Asia leagues FotMob
covers, a few national cups, and the women's top flights). Each
verified entry's `name` matches the upstream `name` field FotMob
serves, so admins can spot mismatches at a glance.

If the seed has `enabled` rows that FotMob no longer serves (e.g.
the upstream platform dropped a league), the scraper silently returns
no matches for those rows — they cost nothing but a rate-limited HTTP
call.

### Adding a league

Two paths. The admin UI at `/#/scraper-leagues` has an "Add new" button
that hits `POST /api/web/v1/scraper-leagues`. If you don't know the
`source_league_id`, search first:

```bash
curl -sH "Authorization: Bearer $ADMIN_TOKEN" \
  "https://your.host/api/web/v1/scraper-leagues/search?q=premier"
```

The search endpoint hits FotMob's `/api/searchapi/suggest?term=…` and
filters for `type=league` entries. It returns `{data: [{source,
source_league_id, name, country, sport}, …]}`. Pick one and POST it
back; the service layer normalizes the country to uppercase and
defaults `enabled=true` so the next cron tick picks it up.

### Removing / disabling

```bash
curl -X PATCH -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}' \
  "https://your.host/api/web/v1/scraper-leagues/<id>"
```

or hit `DELETE` to soft-delete (the row stays in the table with
`deleted_at` set; the unique index keeps the seed idempotent on
restart).

### Editing the seed

The curated list lives in `internal/seeder/defaults.go`. Each new
FotMob platform upgrade may assign different ids; if you discover a
wrong id during scrape, add a comment with the verified id and PR it.
A wrong id is harmless (no matches) but eats one rate-limited HTTP call
per cron tick.

## User roles

Roles are `user` (default) and `admin`. The first user becomes admin
via the bootstrap invitation flow. Admins can change anyone's role
through `/#/users` → Edit → Role dropdown, which hits
`PUT /api/web/v1/users/:id/role`. The handler refuses to demote the
last admin (`409 Conflict` with `cannot demote the last
administrator`) so the system can't lock itself out of the admin API.

## Team logos

The `/#/events` page renders team logos via
`/api/app/v1/teams/logo/<team_id>` — a local proxy that reads from
`IMAGE_STORAGE_PATH`/teams/<id>. The download path is async (via
`LogoScheduler`) and uses the source URL's origin as the `Referer`
header so the request matches the upstream CDN's expectation
(FotMob's CDN rejects requests with the wrong `Referer`).

When a logo file is missing the proxy returns 404; the frontend
`TeamBadge` component catches that and falls back to a circular
initials chip, so the table row stays visually complete instead of
rendering blank space. **A fallback chip is not a broken deployment
signal** — it's the expected UI state until the logo scheduler
finishes downloading the file.
