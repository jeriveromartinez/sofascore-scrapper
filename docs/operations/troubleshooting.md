# Troubleshooting

**Audience:** on-call operator. Pairs with
[`runbook.md`](runbook.md) (alert responses) and
[`verify-scraper-output.md`](verify-scraper-output.md) (data-quality
diagnostics). This page covers **symptoms**: what you see, what is
likely happening, how to confirm, how to recover.

## 30-second triage

Before diving into a specific symptom, run the four-line check:

```bash
sudo systemctl is-active iptv.service            # 1. is the service alive?
sudo journalctl -u iptv.service -n 50 --no-pager # 2. last 50 lines of logs
curl -fs http://127.0.0.1:8080/health/live       # 3. process answering HTTP?
curl -fs http://127.0.0.1:8080/health/ready      # 4. dependencies OK?
```

`/health/live` returning 200 but `/health/ready` returning 503 means the
process is alive but a dependency (DB or Redis) is unreachable — skip
straight to [DB or Redis unreachable](#db-or-redis-unreachable).
`/health/live` returning non-200 means the process is gone or stuck —
go to [Service won't start or stays down](#service-wont-start-or-stays-down).
If both are 200, the platform itself is healthy; the failure is
upstream — go to [Scraper returns nothing or wrong data](#scraper-returns-nothing-or-wrong-data).

For `/health/ready` semantics, see
[`health.go`](../../internal/server/health.go); it pings MariaDB and
Redis before returning 200.

---

## Service

### Service won't start or stays down

**Symptoms.** `systemctl status iptv.service` shows `failed` or
`activating (auto-restart)`. `journalctl -u iptv.service -n 50` shows
the process exiting within ~1s of starting.

**Likely causes (most common first).**

1. **`JWT_SECRET` missing or shorter than 32 chars.**
   The binary refuses to start without it.
   `config.Load` returns `ErrJWTSecretRequired` (`internal/config/config.go:15`)
   or `ErrJWTSecretTooShort` (`config.go:16`).
   *Confirm:* `journalctl` shows `JWT_SECRET is required` or
   `JWT_SECRET is too short`.
   *Fix:* set `JWT_SECRET=` to a 32+ character value in
   `/etc/iptv/env`, then `sudo systemctl restart iptv.service`.

2. **MariaDB unreachable.**
   *Confirm:* log line `Error 1045: Access denied for user` or
   `connect: connection refused`.
   *Fix:* check `DB_HOST`, `DB_PORT`, `DB_PASSWORD` in
   `/etc/iptv/env`. If MariaDB crashed, restart it with
   `sudo systemctl restart mariadb`.

3. **`/etc/iptv/env` not loaded.** Running the binary by hand yields
   `JWT_SECRET is required` even though the file is correct.
   *Fix:* the systemd unit loads the env file automatically. When
   running the binary from a shell, source it first:
   `set -a; . /etc/iptv/env; set +a`.

4. **Port already in use.** Another process holds `:8080`.
   *Confirm:* `sudo ss -lntp | grep 8080`.
   *Fix:* change `API_ADDR` in `/etc/iptv/env` to a free port and
   restart.

### Service restart-loops every 5s

**Symptoms.** `systemctl status` shows `active (running)` and
`activating (auto-restart)` cycling. `journalctl` shows the same error
on every attempt. The `RestartSec` is 5s in the unit
(`deployments/package/iptv.service`).

**Likely causes.** Same as a clean startup failure, but now the
process keeps trying. The most common cause is a credential rotation
that wasn't paired with the env file: you changed the MariaDB user
password in `mariadb` but left `/etc/iptv/env` pointing at the old one.
The service starts, MariaDB rejects the handshake, the process exits,
`RestartSec=5s` fires, repeat.

*Fix:*

1. `sudo mariadb -uroot` and rotate the MariaDB user: `ALTER USER 'iptv'@'localhost' IDENTIFIED BY '<new password>';`
2. Edit `/etc/iptv/env` so `DB_PASSWORD=<new password>` matches.
3. `sudo systemctl restart iptv.service`.

### Bootstrap invitation is refused

**Symptoms.** Running `bootstrap-invitation` prints
`users table is not empty, refusing to bootstrap invitation`.

*Confirm:* `sudo mariadb -uroot iptv -e "SELECT COUNT(*) FROM users"`
returns `> 0`.

*Fix options:*

- **Log in as the existing admin** and use `/#/users` to invite
  additional operators.
- **Reset to a fresh database** if you are sure no production users
  exist: `sudo systemctl stop iptv.service && sudo mariadb -uroot -e "DROP DATABASE iptv"`, then restart the service. The
  postinst does not run on a manual DB drop; you recreate the schema
  by starting the service (GORM AutoMigrate).

The invitation subcommand is single-use: a successfully issued token
expires in 24 hours (Redis TTL).

---

## Database (MariaDB)

### DB or Redis unreachable

**Symptoms.** `/health/ready` returns 503; `/health/live` returns 200.
`journalctl` is full of `dial tcp 127.0.0.1:3306: connect: connection refused`
or `redis: connection refused`.

*Confirm:* on the host, `mariadb-admin ping -h 127.0.0.1` and
`redis-cli ping`.

*Fix:*

- **MariaDB down:** `sudo systemctl status mariadb` →
  `sudo systemctl restart mariadb`. If it crashes again, check
  `journalctl -u mariadb -n 200` for the actual cause.
- **Redis down:** `sudo systemctl status redis-server` →
  `sudo systemctl restart redis-server`. The platform handles Redis
  outages as **soft failures** — see
  [`runbook.md` → Redis outage](runbook.md#redis-outage-behavior).

### DB connection pool exhausted

**Symptoms.** Prometheus alert `scores365_db_connection_pool_exhausted`
fires. `journalctl` shows `wait_for_connection: too many connections already`
or queries hanging for >2 minutes.

*Confirm:*

```sql
SHOW STATUS LIKE 'Max_used_connections';
SHOW STATUS LIKE 'Threads_connected';
SHOW VARIABLES LIKE 'max_connections';
```

*Fix:*

1. Check whether a single query is the blocker:
   `SHOW FULL PROCESSLIST` — kill long-running ones with
   `KILL <id>;`.
2. If exhausted under normal load, raise MariaDB's `max_connections`
   (default 151) and/or lower `DB_MAX_OPEN_CONNS` per backend.
   The default `DB_MAX_OPEN_CONNS=25` × 3 backends = 75 active
   connections; with cron bursts and the cron lock retrying, you can
   exceed 151 with three backends.
3. If `Max_used_connections` keeps growing without bound, you have a
   goroutine leak; restart the service to recover, then look for
   recent code changes that opened connections without closing them.

### Migration fails on boot

**Symptoms.** `iptv.service` exits with a GORM error during
AutoMigrate (`Error 1060: Duplicate column name` etc.).

*Fix:*

- `GORM` AutoMigrate is **additive only** (see
  [`rollback.md` → Schema policy](rollback.md#schema-policy)).
  It does not drop or alter columns; it only adds missing tables and
  columns. A column that already exists but with a different type
  is left untouched, so a deploy that renames a column in the model
  but the DB still holds the old shape will not break AutoMigrate — it
  will silently mismatch.
- A genuinely destructive change (rename, type change, drop) must be
  applied manually to the live DB before deploying the new binary,
  then left in place.

---

## Redis

### Redis returned, but rate limits or invitations are missing

**Symptoms.** End users report being able to log in without rate
limiting kicking in; the bootstrap invitation link "expires"
immediately.

*Confirm:* `redis-cli INFO memory | grep maxmemory`. If
`maxmemory_policy` is `allkeys-lru` (the common default), Redis
silently evicts keys under memory pressure — including the rate-limit
counters (`ratelimit:<sha256>`) and invitation tokens
(`auth:invitation:<sha256>`). Those are the base key names;
`REDIS_KEY_PREFIX`, when set, is prepended to them.

*Fix:* set `maxmemory-policy noeviction` in `/etc/redis/redis.conf`
and restart redis-server. The application expects rate-limit and
invitation keys to persist for the configured TTL.

### Multi-instance: another tenant's notifications fire on this tenant

**Symptoms.** Operators see push notifications on devices that don't
belong to them.

*Confirm:* two backend stacks (e.g. staging + production) sharing one
Redis. Push-timer keys are namespaced by `REDIS_KEY_PREFIX`. When the
prefix is empty (the default), the keys collide: one stack's timer
runner fires the other stack's scheduled pushes.

*Fix:* set `REDIS_KEY_PREFIX=stg-` (or `prod-`) on each backend and
restart. Use a separate Redis instance per stack when you can.

### Redis outage

*Symptoms.* Same as [DB or Redis unreachable](#db-or-redis-unreachable)
but with Redis specifically.

*Behavior.* The backend uses Redis for two things:

- **Distributed locks** for scheduled jobs. If Redis is down,
  scheduled jobs are skipped on this backend and picked up by
  another. When Redis returns, locks resume normally.
- **Realtime WebSocket fanout** and rate limiting. Clients see a
  `WebSocketChannelException` and reconnect. No data is lost.

No manual intervention is needed. See
[`runbook.md` → Redis outage](runbook.md#redis-outage-behavior).

---

## Storage / Disk

### APK storage disk full

**Symptoms.** Prometheus alert `iptv_apk_storage_full` (when
configured) or disk-full alarms. New APK uploads fail with
`no space left on device`; logo downloads stop.

*Confirm:* `du -sh /opt/iptv/apk_storage/* | sort -h | tail -20` —
old, half-uploaded chunks accumulate in
`/opt/iptv/apk_storage/`.

*Fix:* the cleanup cron (`internal/apk/cleanup.go`) runs every
15 minutes and removes the chunks of upload sessions that have
expired — the session TTL is 24 hours (`uploadSessionTTL` in
`internal/apk/upload_state.go`). Chunks live in
`/opt/iptv/apk_storage/chunks/<upload_id>/chunk-<n>`. Wait for the
next sweep; if it has not recovered, prune manually:
`find /opt/iptv/apk_storage/chunks -mindepth 1 -maxdepth 1 -type d -mmin +1440 -exec rm -rf {} +`.
Assembled builds are published straight into
`/opt/iptv/apk_storage/<package>-<version>.apk` — there is no
`finished/` subdirectory — so the policy is keep-the-active-APK-only:
`rm /opt/iptv/apk_storage/<package>-<old-version>.apk` is safe as
long as no device is still downloading it (check `apk_versions.active`).

### Image storage disk full

**Symptoms.** `/opt/iptv/image_storage` approaches 100% full. Team
logos stop downloading; the `TeamBadge` Vue component falls back to
initials chips — but **a fallback chip is the expected UI state until
the scheduler catches up**, not a broken-deployment signal.

*Fix:* `du -sh /opt/iptv/image_storage/* | sort -h | tail -20` —
identify the largest leagues by storage footprint. Optionally remove
logos for leagues you no longer cover. The logo scheduler is
self-healing on next tick.

### Logo downloads fail with `Referer` mismatch

**Symptoms.** Operators see team logos with initials; the backend log
shows `events: failed to download logo for team` repeatedly for a
specific team or batch.

*Confirm:*

```bash
sudo journalctl -u iptv.service | grep "failed to download logo"
```

*Fix:* upstream CDNs (FotMob, 365scores) check the `Referer` header
on logo requests. If an upstream changes its expected `Referer`, all
logos silently revert to initials chips. The handler has a fallback
(`internal/events/logo_store.go`, `TeamLogoSourceURL`) that consults the
CDN's URL allow-list; check whether the URL pattern changed upstream before
treating this as an env-var problem.

---

## Auth

### `JWT_SECRET is too short`

The binary enforces `MinJWTSecretLength = 32` (`internal/config/config.go:21`).
Edit `/etc/iptv/env` so `JWT_SECRET` is at least 32 bytes
(`openssl rand -hex 32 | head -c 32` works) and restart.

### Invitation token never arrives / expired immediately

**Symptoms.** Operator runs `bootstrap-invitation`, prints the token,
but `POST /api/web/v1/users/register` returns 401 / 404 / `invalid token`.

*Confirm:* `redis-cli KEYS 'auth:invitation:*'` (or `KEYS '*'`
depending on your `REDIS_KEY_PREFIX`). If the key is missing or has
no TTL, Redis lost it (see [Redis returned, but rate limits or invitations are missing](#redis-returned-but-rate-limits-or-invitations-are-missing)).

### Login returns 401 even though the password is right

*Confirm:* check `JWT_SECRET` length first; the application rejects
short secrets. Then check whether `/api/web/v1/users/login` got a
JSON body — this endpoint is **protobuf only**, JSON silently
yields zero-valued fields.

---

## Scraping

### Scraper returns nothing or wrong data

**Symptoms.** `events` table is empty or growing slowly;
`scraper_leagues` table has rows with `enabled=true`.

*Confirm in order:*

1. Is the cron running?
   `sudo journalctl -u iptv.service | grep 'scrape day'`
2. Is the upstream reachable from this host?
   ```bash
   DAY=$(date -u +%d/%m/%Y)
   curl -fs -H 'Referer: https://www.365scores.com/' \
     "https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=$DAY&endDate=$DAY&onlyMajorGames=false&withTop=true" | head -c 200
   ```
   The upstream rejects requests without the `Referer` header.
3. Is the league catalog doing its job? See
   [`verify-scraper-output.md` → Is the league catalog doing its job?](verify-scraper-output.md#3-is-the-league-catalog-doing-its-job).

### Scraper rate-limited (429)

**Symptoms.** Logs show `scrape: 429 from /web/games/allscores/` then
`fetch error: rate limited`.

*Confirm:* `curl -I -H 'Referer: https://www.365scores.com/' \
  'https://webws.365scores.com/web/games/allscores/?appTypeId=5&langId=29&timezoneName=UTC&userCountryId=116&startDate=21/10/2026&endDate=21/10/2026&onlyMajorGames=false&withTop=true'`
returns `HTTP 429` repeatedly from the backend host.

*Behavior:* The HTTP client retries with exponential backoff
(1s, 2s, 4s, 8s, 16s) before giving up. Short 429 bursts within a
60-second window are absorbed by the in-memory cache; longer bursts
exhaust the retries and the tick fails. Sustained 429s indicate a
real rate-limit; escalate before considering a code change.

### Discovery didn't add new leagues

**Symptoms.** Operator expects a new 365scores league to appear in
`scraper_leagues`, but it is not there on first boot of the day.

*Confirm:*

```bash
sudo mariadb -uroot scores365 -e \
  "SELECT source, sport, COUNT(*) FROM scraper_leagues WHERE source='scores365' GROUP BY sport;"
```

*Fix:* Discovery runs **once at boot only**. The current mechanism
(`internal/scraper/catalog/discovery.go`) makes a single
`/web/competitions/` call against the upstream with a 45-second
timeout. There is no scheduled re-discovery; **restart the backend
to pick up new leagues**. If the endpoint is unreachable, `curl -I
https://webws.365scores.com/web/competitions/?sports=2,3,5,7,8,9` from
the host to confirm reachability.

---

## Push / realtime

### Push timer never fires

**Symptoms.** Operators schedule push notifications in the dashboard;
they never reach devices.

*Confirm:* `sudo journalctl -u iptv.service | grep 'push timer'`
shows no recent "tick" entries. Check `PUSH_TIMER_TICK_INTERVAL` and
`PUSH_TIMER_BATCH_LIMIT` (both env vars; defaults `5s` and `100`).

*Common cause:* in a multi-instance deploy, both backends share one
Redis with empty `REDIS_KEY_PREFIX`; one backend's runner dequeues
the other backend's keys (see
[Multi-instance: another tenant's notifications fire on this tenant](#multi-instance-another-tenants-notifications-fire-on-this-tenant)).

### WebSocket clients keep disconnecting

**Symptoms.** Devices reconnect every few seconds; realtime UI shows
flickering.

*Confirm:* `sudo journalctl -u iptv.service | grep 'websocket:'`
shows repeated close / open cycles.

*Fix:* the backend handles WebSocket disconnect by design (clients
auto-reconnect). If you see persistent disconnect storms during
peak traffic, check `/health/ready` (which includes Redis) — the
backend's Pub/Sub fanout fails when Redis is unreachable.

---

## APK / uploads

### Chunked upload stalls mid-way

**Symptoms.** Client sees uploads stop at chunk N of M with `0 B/s`.

*Confirm:*

- The client's local upload state — does it have a `upload_id`?
  Without it, the server cannot resume; the client must restart.
- `/opt/iptv/apk_storage` is the disk path the upload resumes
  against. Chunks are written to
  `/opt/iptv/apk_storage/chunks/<upload_id>/chunk-<n>`. Verify with
  `ls /opt/iptv/apk_storage/chunks/` — if the directory is missing,
  the server had to recreate it (a `mkdir` failure typically shows in
  `journalctl`).

*Fix:* ask the client to retry from chunk 1 with a fresh
`POST /uploads/begin`. Old partial chunks are cleaned up by
the 15-minute job (see [APK storage disk full](#apk-storage-disk-full)).

---

## Frontend / dashboard

### Dashboard 404 / API-only

**Symptoms.** Loading `/` returns 404 or shows raw API output.

*Confirm:* `/opt/iptv/web/dist/index.html` exists and is readable
by the `iptv` system user.

*Fix:* the binary serves `web/dist` from disk, relative to its
working directory. If the distribution is missing, you built a
backend binary without the frontend step. Run the `.deb` package
build (see [`deb-package.md`](deb-package.md)) which always runs the
`build-vue` stage first.

### Protobuf endpoint accepts JSON, then rejects as missing fields

**Symptoms.** `POST /api/web/v1/users/register` with a JSON body
returns `{"error":"email and password are required"}`.

*Confirm:* `Content-Type` header. Auth endpoints require
`application/x-protobuf` with the binary wire encoding from
`proto/api.proto`. JSON is silently accepted by `proto.Unmarshal`
and yields zero-valued fields, so the handler thinks the request is
empty.

*Fix:* use `scripts/auth_smoke/` (Go program in this repo) or send
the protobuf binary directly. The `/api/app/v1/crash-report`
endpoint is the documented exception (JSON only).

---

## Detection catalog — known production failure modes

This is a non-exhaustive list of failure modes documented or fixed
during real operations. Use it to triage an unfamiliar symptom.

1. **`iptv.service` restart-loop after credential rotation.** Always
   pair a MariaDB password rotation with editing `/etc/iptv/env` and
   restarting the service. The `RestartSec=5s` will mask the real
   error in `journalctl`.
2. **`/etc/iptv/env` silently keeps an old `DB_USER` after upgrade.**
   The `.deb` postinst never overwrites an existing env file, so a
   hand-edited `DB_NAME` vs the created `iptv` database yields
   `Unknown database` forever. Edit directly when needed.
3. **Disk full on `/opt/iptv/image_storage`.** Team logos accumulate
   one PNG per team. `du -sh /opt/iptv/image_storage/*` to find the
   largest leagues; team logos are safe to remove when no longer
   wanted (the scheduler re-fetches as needed).
4. **MariaDB `max_connections` exhausted behind 3 instances.**
   `DB_MAX_OPEN_CONNS=25` × 3 backends + cron bursts can exceed
   MariaDB's default 151. Either lower per-backend limits or raise
   `max_connections` in `mariadb-server`.
5. **Redis eviction silently drops rate-limit + invitation keys.**
   Set `maxmemory-policy noeviction`. Otherwise, on memory pressure,
   Redis evicts `ratelimit:*` and `auth:invitation:*` keys.
6. **`/health/ready` 503 while `/health/live` is 200.** Readiness
   pings Redis. The most common alert is a Redis blip — check
   `journalctl -u redis-server`.
7. **Logo downloads blocked by CDN `Referer` mismatch.** Upstreams
   change expected `Referer` headers; the handler's fallback
   (per-source URL allow-list) is the safety net.
8. **Events arrive with wrong `source` after a FotMob upstream ID
   change.** `scraper_leagues` rows with stale IDs silently drop
   rows. Use the catalog-vs-events LEFT JOIN from
   `verify-scraper-output.md` to spot leagues going dark.
9. **Push timers never fire after a multi-instance deploy.** Empty
   `REDIS_KEY_PREFIX` causes cross-stack key collisions. Set the
   prefix and restart.
10. **Timezone drift shifts "today" between FotMob and 365scores.**
    `FOTMOB_TIMEZONE` defaults to `Europe/Paris`, `SCORES365_TIMEZONE`
    to `UTC`. A day-boundary mismatch yields duplicated or missing
    fixtures with no log line. If you change one, change the other
    to align.
