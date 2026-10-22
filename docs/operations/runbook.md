# Operations Runbook

Day-to-day procedures for the on-call operator. Pair this with
[`README.md`](README.md) (entry point, env vars, checklist),
[`troubleshooting.md`](troubleshooting.md) (symptom-first catalog),
[`rollback.md`](rollback.md) (recovery procedures) and
[`verify-scraper-output.md`](verify-scraper-output.md) (data-quality
diagnostics).

## Alert response

The four Prometheus alerts the platform ships are defined in
[`deployments/prometheus/alerts.yml`](../../deployments/prometheus/alerts.yml).
Their alert-rule names are the canonical keys.

| Alert | First action |
|---|---|
| `scores365_health_probe_failure` | The `/health/ready` probe has been failing for 2 minutes. `curl -fs http://127.0.0.1:8080/health/ready` from the host. If 503, jump to [Troubleshooting → DB or Redis unreachable](troubleshooting.md#db-or-redis-unreachable). |
| `scores365_scrape_failure_rate` | `>5%` error ratio over 5 minutes. `curl -fs -H 'Referer: https://www.365scores.com/' 'https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=21/10/2026&endDate=21/10/2026&onlyMajorGames=false&withTop=true' \| head -c 200` from the host. 200 = upstream OK; 503/429 = upstream degraded; 404 = endpoint removed (escalate). |
| `scores365_goroutine_leak_high` | `go_goroutines` trending up by >5/10m and currently >200. Restart the service to recover: `sudo systemctl restart iptv.service`. Then look for recent code changes that opened goroutines without canceling them. |
| `scores365_db_connection_pool_exhausted` | `sql_in_use_connections == sql_open_connections` and waiters accumulating. `SHOW FULL PROCESSLIST` to find blocking queries; `KILL <id>` to clear them. If chronic, lower `DB_MAX_OPEN_CONNS` per backend or raise MariaDB's `max_connections`. |

### Alert: scores365_health_probe_failure

`/health/ready` has been failing for 2 minutes. The readiness probe
(`internal/server/health.go`) checks both MariaDB and Redis.

```bash
curl -fs http://127.0.0.1:8080/health/ready
sudo journalctl -u iptv.service -n 50 --no-pager
```

A 503 means a dependency is unreachable — jump to
[Troubleshooting → DB or Redis unreachable](troubleshooting.md#db-or-redis-unreachable).
A 200 means the probe target itself is stale; check the blackbox
scrape configuration.

### Alert: scores365_scrape_failure_rate

More than 5% of scrape requests failed over 5 minutes. Check the
upstream from the host:

```bash
curl -fs -H 'Referer: https://www.365scores.com/' \
  'https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=21/10/2026&endDate=21/10/2026&onlyMajorGames=false&withTop=true' | head -c 200
```

`200` = upstream OK; `503`/`429` = upstream degraded; `404` = the
endpoint moved (escalate). The client retries with exponential
backoff (1s, 2s, 4s, 8s, 16s) and absorbs short 429 bursts with its
60-second in-memory cache.

### Alert: scores365_goroutine_leak_high

`go_goroutines` is trending up by more than 5 per 10 minutes and is
now above 200. Restart to recover:

```bash
sudo systemctl restart iptv.service
```

Then look for recent code changes that opened goroutines without
cancelling them. A goroutine leak in the scraper or the realtime
hub shows up here before it shows up anywhere else.

### Alert: scores365_db_connection_pool_exhausted

All SQL connections are in use and at least one waiter has
accumulated in the last 2 minutes.

```bash
sudo mariadb -uroot -e "SHOW FULL PROCESSLIST"
sudo mariadb -uroot -e "SHOW STATUS LIKE 'Max_used_connections'"
```

`KILL <id>` clears a blocking query. If it recurs under normal
load, lower `DB_MAX_OPEN_CONNS` per backend or raise MariaDB's
`max_connections`.

## Redis outage behavior

The backend uses Redis for two things: distributed locks for scheduled
jobs and the realtime WebSocket fanout. Both are designed to fail soft:

- **Distributed locks** — if Redis is down, scheduled jobs are
  skipped on this backend. Other backends still pick up the work.
  When Redis returns, locks resume normally.
- **Realtime WebSocket** — clients see a `WebSocketChannelException`
  and reconnect. The backend logs the disconnect; no data is lost.

When Redis recovers, no manual intervention is needed.

For production failure modes around Redis eviction and multi-instance
prefix collisions, see
[Troubleshooting → Redis](troubleshooting.md#redis).

## Upload cleanup

Old APK upload chunks accumulate in `APK_STORAGE_PATH` if clients
abort uploads mid-way. The cleanup job runs every 15 minutes and
removes the chunks of upload sessions that have expired — the session
TTL is 24 hours (`uploadSessionTTL` in
`internal/apk/upload_state.go`). Chunks live in
`<APK_STORAGE_PATH>/chunks/<upload_id>/chunk-<n>`. If the job
itself fails, the next tick retries; nothing else to do unless the
disk is full.

## Counter recovery

APK download counters are flushed every 15 minutes. The flush is
idempotent: `download_counter_flushes` holds only the `batch_id`
primary key, and a batch that is already recorded is skipped
(`ON CONFLICT DO NOTHING`). If the backend is killed mid-flush, the
pending counter batch is still in Redis under
`apk:downloads:pending:<batch_id>`; on the next start
`ReprocessOrphans` scans for those keys and re-applies the deltas.

Manual check:

```bash
sudo mariadb -uroot iptv -e "SELECT * FROM download_counter_flushes LIMIT 5"
redis-cli KEYS 'apk:downloads:pending:*'
```

If a pending batch is still in Redis, the next automatic flush
picks it up.

## Cache invalidation

There is no application cache to invalidate manually. Static assets
are served from `web/dist` on disk, relative to the service's working
directory; the cache headers are `Cache-Control: public, max-age=...`
based on file hashes, so clients pick up new builds automatically.

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
redis-cli INFO memory
```

## First Boot

On a fresh database (where `users` is empty), the server auto-seeds a
default administrator:

- **Email:** `admin@local`
- **Password:** the value of `DEFAULT_ADMIN_PASSWORD`; when that
  variable is unset the seeder generates a random password and never
  prints it
- **Role:** `admin`

> **Set `DEFAULT_ADMIN_PASSWORD` before the first boot** if you want a
> known credential — the seeder uses that value as-is. When the
> variable is unset the generated password is not logged, so the seeded
> account cannot be logged into; the way to register the first operator
> is the bootstrap-invitation flow below.

To pre-apply schema and exit (no server), on a host where the binary
lives at `/opt/iptv/iptv`:

```bash
sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv migrate'
```

This runs `AutoMigrateAll` and the seeder, then exits 0.

To skip migration and seeding entirely (when managing schema
externally):

```bash
SKIP_MIGRATE=true /opt/iptv/iptv
```

## Bootstrap via invitation

If you prefer that the first operator register through the normal
invitation flow rather than logging in as `admin@local`, run:

```bash
sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation'
```

The command prints the invitation token (also visible in the logs)
and refuses to run once `users` already has a row, so it must run
before the server's first normal boot. Start the server afterward
and the first human registers at `/register` using the token; that
account becomes the sole admin via the existing first-user rule.

The token has a Redis TTL of 24 hours (`internal/auth/invitation.go`)
and is single-use; see
[Troubleshooting → Invitation token never arrives](troubleshooting.md#invitation-token-never-arrives--expired-immediately)
for the failure modes around lost keys.

## Working with branches

Use git worktrees inside the project, under `.worktrees/` at the
repo root:

```bash
git worktree add .worktrees/<branch> -b <branch>       # new branch
git worktree add .worktrees/<branch> <existing-branch>  # existing
git worktree remove .worktrees/<branch>                 # cleanup
```

`.worktrees/` is gitignored. When the branch is merged or abandoned,
remove the worktree with `git worktree remove` rather than deleting
the folder manually.

## Scraper catalog

The scraper only runs against leagues present in the `scraper_leagues`
table with `enabled = true`. On a fresh DB the server auto-seeds a
curated list of leagues whose `source_league_id` was verified against
upstream on 2026-09-13. As of that date the seed has 41 entries (top
5 European leagues + second divisions + rest of Europe top flights,
the main LATAM / USA / Asia leagues FotMob covers, a few national
cups, and the women's top flights). Each verified entry's `name`
matches the upstream `name` field, so admins can spot mismatches at
a glance.

If the seed has `enabled` rows that upstream no longer serves, the
scraper silently returns no matches for those rows — they cost nothing
but a rate-limited HTTP call.

### Adding a league

The catalog is managed through the API only — there is no admin UI
page for it. Create a league with `POST /api/web/v1/scraper-leagues`.
If you don't know the `source_league_id`, search first:

```bash
curl -sH "Authorization: Bearer $ADMIN_TOKEN" \
  "https://your.host/api/web/v1/scraper-leagues/search?q=premier"
```

The search endpoint hits FotMob's
`/api/searchapi/suggest?term=…` and filters for `type=league`
entries. It returns `{data: [{source, source_league_id, name,
country, sport}, …]}`. Pick one and POST it back; the service layer
normalizes the country to uppercase and defaults `enabled=true` so
the next cron tick picks it up.

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
upstream platform upgrade may assign different ids; if you discover a
wrong id during scrape, add a comment with the verified id and PR
it. A wrong id is harmless (no matches) but eats one rate-limited
HTTP call per cron tick.

## User roles

Roles are `user` (default) and `admin`. The first user becomes admin
via the bootstrap invitation flow. Admins can change anyone's role
through `/#/users` → Edit → Role dropdown, which hits
`PUT /api/web/v1/users/:id/role`. The handler refuses to demote the
last admin (`409 Conflict` with `cannot demote the last
administrator`) so the system can't lock itself out of the admin
API.

## Team logos

The `/#/events` page renders team logos via
`/api/app/v1/teams/logo/<team_id>` — a local proxy that reads from
`IMAGE_STORAGE_PATH`/teams/<id>. The download path is async (via
`LogoScheduler`) and uses the source URL's origin as the `Referer`
header so the request matches the upstream CDN's expectation.

When a logo file is missing the proxy returns 404; the frontend
`TeamBadge` component catches that and falls back to a circular
initials chip, so the table row stays visually complete instead of
rendering blank space. **A fallback chip is the expected UI state
until the logo scheduler catches up** — not a broken-deployment
signal. See
[Troubleshooting → Logo downloads fail with `Referer` mismatch](troubleshooting.md#logo-downloads-fail-with-referer-mismatch)
for the failure modes around CDN rejection.
