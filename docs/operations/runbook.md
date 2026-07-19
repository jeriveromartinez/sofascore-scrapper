# Operations Runbook

## Environment Matrix

| Environment | Compose File | DB | Redis | Instances | LB | Purpose |
|---|---|---|---|---|---|---|
| **dev** | `compose.dev.yml` | MariaDB 10.11 (persistent) | Redis 7 (persistent) | 1 | none | Local development |
| **test** | `compose.test.yml` | MariaDB 10.11 (tmpfs/ephemeral) | Redis 7 (tmpfs/ephemeral) | 1 + init | none | CI / integration tests |
| **multi** | `compose.multi.yml` | MariaDB 10.11 (persistent) | Redis 7 (persistent) | 3 | nginx 1.27 | Staging / load-test |
| **prod** | Native binary + user systemd | MariaDB (persistent) | Redis (persistent) | 1 | external proxy/LB | Production on runner `iptv` |

### Key Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `DB_HOST` | `localhost` | Yes | MariaDB hostname |
| `DB_PORT` | `3306` | No | MariaDB port |
| `DB_USER` | `root` | No | MariaDB user |
| `DB_PASSWORD` | *(empty)* | Yes (prod) | MariaDB password |
| `DB_NAME` | `sofascore` | No | Database name |
| `DB_MAX_OPEN_CONNS` | `25` | No | Max open DB connections |
| `DB_MAX_IDLE_CONNS` | `10` | No | Max idle DB connections |
| `DB_CONN_MAX_LIFETIME` | `30m` | No | Max connection lifetime |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | No | Max connection idle time |
| `REDIS_URL` | `redis://localhost:6379/0` | No | Redis connection URL |
| `REDIS_KEY_PREFIX` | *(empty)* | No | Redis key prefix (for shared instances) |
| `REDIS_DIAL_TIMEOUT` | `5s` | No | Redis dial timeout |
| `REDIS_READ_TIMEOUT` | `3s` | No | Redis read timeout |
| `REDIS_WRITE_TIMEOUT` | `3s` | No | Redis write timeout |
| `JWT_SECRET` | *(none)* | **Yes** | JWT signing secret (minimum 32 chars) |
| `API_ADDR` | `:8080` | No | HTTP listen address |
| `PPROF_ADDR` | *(empty)* | No | Pprof debug server address (e.g. `:6060`) |
| `APK_STORAGE_PATH` | `./apk_storage` | No | APK chunk storage directory |
| `IMAGE_STORAGE_PATH` | `./image_storage` | No | Team logo storage directory |
| `SCRAPE_BATCH_SIZE` | `500` | No | Events per scrape batch |
| `SCRAPE_CONCURRENCY` | `8` | No | Concurrent scrape goroutines |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | No | HTTP read header timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | No | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `120s` | No | HTTP idle timeout |

---

## Dashboard URLs

| Path | Auth | Description |
|---|---|---|
| `/` | Public | SPA dashboard (Vue 3, served from `web/dist`) |
| `/health/live` | None | Liveness probe (always returns 200 if process alive) |
| `/health/ready` | None | Readiness probe (checks DB + Redis connectivity) |
| `/metrics` | None | Prometheus metrics endpoint |

### API Endpoints

**App (device-authenticated, rate-limited):**
- `GET /api/app/v1/apk/latest` — latest APK version
- `GET /api/app/v1/apk/download/:token` — download APK by token
- `POST /api/app/v1/devices/register` — register device
- `POST /api/app/v1/playback/start` — log playback start
- `POST /api/app/v1/playback/end` — log playback end
- `POST /api/app/v1/crash` — submit crash report
- `GET /api/app/v1/events` — current live events
- `GET /api/app/v1/logo/:teamId` — team logo image

**Web (JWT-authenticated + rate-limited):**
- `POST /api/web/v1/auth/login` — login
- `POST /api/web/v1/auth/register` — register with invitation
- `POST /api/web/v1/auth/refresh` — refresh JWT
- `POST /api/web/v1/auth/invitation` — create invitation (scoped)
- `GET/POST /api/web/v1/apk` — list / upload APK
- `PUT /api/web/v1/apk/upload/:id` — finalize upload
- `GET/POST/PUT/DELETE /api/web/v1/devices` — device CRUD
- `GET /api/web/v1/playback` — playback logs
- `GET /api/web/v1/stats/*` — content statistics
- `GET/POST /api/web/v1/events` — event management
- `GET/POST/PUT/DELETE /api/web/v1/tournaments` — tournament CRUD
- `GET/POST/PUT/DELETE /api/web/v1/tournaments/assignments` — device-tournament assignments
- `GET/PUT /api/web/v1/tournaments/global-config` — global tournament config
- `GET/POST/PUT/DELETE /api/web/v1/domains` — domain CRUD
- `GET/POST/PUT/DELETE /api/web/v1/users` — user management

---

## Alert Response

### Alert: `sofascore_health_probe_failure`

**Trigger:** `/health/ready` returns non-200.

**Response:**
1. SSH into the affected instance.
2. Check database connectivity: `mariadb-admin ping -h $DB_HOST`
3. Check Redis connectivity: `redis-cli -u $REDIS_URL ping`
4. Review application logs for connection errors.
5. If DB is down: check disk space on DB host (`df -h`), restart MariaDB if safe.
6. If Redis is down: restart Redis; the app will degrade gracefully (see Redis Outage Behavior below).
7. After recovery, verify `GET /health/ready` returns 200.

### Alert: `sofascore_scrape_failure_rate`

**Trigger:** Scrape error rate exceeds 5% in 5 minutes.

**Response:**
1. Check Sofascore availability: `curl -I https://www.sofascore.com`
2. Review scraper error logs for HTTP errors or parsing failures.
3. If Sofascore is blocking: check rate limits, consider increasing `SCRAPE_CONCURRENCY` or decreasing `SCRAPE_BATCH_SIZE`.
4. The next scheduled scrape will catch up automatically (today: every minute, future: every 12 hours at 06:00 and 18:00 UTC).

### Alert: `sofascore_goroutine_leak_high`

**Trigger:** Goroutine count steadily increases without returning to baseline.

**Response:**
1. Connect to pprof endpoint (if enabled): `go tool pprof http://$HOST:6060/debug/pprof/goroutine`
2. Identify blocking goroutines and leaked goroutines.
3. Check Redis lock renewals — a hung lock renewer can leak goroutines.
4. If necessary, perform a rolling restart of affected instances.

### Alert: `sofascore_db_connection_pool_exhausted`

**Trigger:** `DB_MAX_OPEN_CONNS` connections all in use.

**Response:**
1. Check for slow queries: `SHOW PROCESSLIST` on MariaDB.
2. If queries are piling up due to poor performance, consider increasing `DB_MAX_OPEN_CONNS` temporarily.
3. Check for long-running transactions blocking others.

---

## Redis Outage Behavior

The application uses Redis for:

| Feature | Redis Role | Outage Behavior |
|---|---|---|
| **Rate limiting** | Token bucket state | Rate limiter fails open — requests are allowed through. No rate-limiting protection during outage. |
| **Distributed locks** | Redlock-based job locks | `Acquire()` returns `false`. Scheduled jobs (scrape, stats, upload cleanup, download counter flush) gracefully skip. No duplicate job execution, but jobs are paused until Redis recovers. |
| **Event cache** | Current live events | Cache misses fall through to MariaDB. Response time degrades slightly but data remains correct. |
| **Event epoch** | Monotonic counter for cache invalidation | Increments fail. Clients may receive stale data until Redis recovers. |
| **Invitation tokens** | Temporary registration tokens | New user registration is blocked. Existing users can still log in. |
| **Upload state tracking** | Chunked upload progress | Ongoing uploads are lost. Users must restart uploads after Redis recovers. |
| **Download counter** | In-memory download counts (flushed every 15 min) | Download counts for the current 15-minute window are lost. Historical counts in MariaDB are unaffected. |
| **Logging** | Structured log transport | Falls through to stdout. No log loss. |

**Recovery:** When Redis comes back online, all features resume automatically. No manual intervention needed.

---

## Upload Cleanup

### Chunked APK Upload Flow

1. Client calls `POST /api/web/v1/apk/upload` with file metadata → creates upload state in Redis.
2. Client sends chunks via `PUT /api/web/v1/apk/upload/:id/chunk/:index` → writes chunk files to `APK_STORAGE_PATH`.
3. Client finalizes: `PUT /api/web/v1/apk/upload/:id` → assembles chunks, persists to database.

### Automatic Cleanup (cron: `*/15 * * * *`)

The scheduler runs an upload cleanup job every 15 minutes. It removes:
- Upload state entries in Redis that have no corresponding chunk files.
- Orphan chunk files that have no corresponding Redis state.
- Incomplete uploads older than the Redis TTL (expired keys).

**Manual cleanup:**
```bash
# Remove all upload state from Redis
redis-cli -u $REDIS_URL --scan --pattern "upload:*" | xargs redis-cli -u $REDIS_URL DEL

# Remove orphan chunk files
find $APK_STORAGE_PATH -type f -name "*.chunk" -mtime +1 -delete
```

---

## Counter Recovery

### Download Counter Architecture

Download counts are tracked in Redis (atomic increments) and flushed to MariaDB every 15 minutes via a cron job gated by a distributed lock (`scheduler:lock:apk-downloads:flush`).

**Flush flow:**
1. `counter.Flush(ctx)` reads Redis counter values, writes them to `apk_versions.total_downloads`.
2. Each flush records a `batch_id` in `download_counter_flushes` table.
3. `counter.ReprocessOrphans(ctx)` detects orphan counters (written to Redis but missing flush record) and re-processes them.

### Manual Counter Recovery

```bash
# Check current Redis counter values
redis-cli -u $REDIS_URL KEYS "apk:downloads:*" | while read key; do
  echo "$key: $(redis-cli -u $REDIS_URL GET $key)"
done

# Force a flush (needs running instance)
curl -X POST http://localhost:8080/admin/flush-downloads

# Reset a specific counter (after corruption)
redis-cli -u $REDIS_URL DEL "apk:downloads:<version_id>"

# Rebuild totals from raw data (direct DB):
# Counts are additive; set total_downloads to the sum of all flush batch records.
```

---

## Cache Invalidation

### Event Cache

Live events are cached in Redis under `events:current`. The cache is invalidated by:

1. **Epoch-based invalidation:** Each time events change (scrape completes, tournament assignments change, global config changes), the epoch counter in Redis (`events:epoch`) is incremented. Clients include the epoch in their request; if the server epoch is higher, the client re-fetches.

2. **Manual invalidation:**
```bash
# Force cache refresh by bumping the epoch
redis-cli -u $REDIS_URL INCR "events:epoch"

# Delete the current event cache entirely
redis-cli -u $REDIS_URL DEL "events:current"
```
The next scrape cycle will repopulate the cache automatically.

### Rate Limit Reset

```bash
# Reset rate limit for a specific IP
redis-cli -u $REDIS_URL KEYS "ratelimit:*" | while read key; do
  redis-cli -u $REDIS_URL DEL "$key"
done
```

---

## Destructive Stats Reset Warning

**Migration `000003_reset_playback_and_stats.up.sql` is DESTRUCTIVE.**

It executes:
```sql
DELETE FROM `content_stats`;
DELETE FROM `playback_logs`;
```

**Before running this migration in production:**
1. Take a full database backup: `mysqldump -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME > backup_$(date +%Y%m%d_%H%M%S).sql`
2. Verify the backup: `grep -c "INSERT INTO" backup_*.sql`
3. Export content_stats and playback_logs separately for analysis.
4. Schedule a maintenance window.

**Rollback of this migration is NOT possible** — the `.down.sql` is a no-op. Once data is deleted, only a backup can restore it. See [rollback.md](rollback.md).

---

## Health Check Matrix

| Endpoint | Checks | HTTP 200 Condition |
|---|---|---|
| `/health/live` | None (process alive) | Always |
| `/health/ready` | DB (`SELECT 1`), Redis (`PING`) | Both pass |

If `/health/ready` fails, the nginx load balancer in `compose.multi.yml` will not route traffic to the instance. Health checks are configured with:
- Interval: 15s
- Timeout: 5s
- Retries: 3
- Start period: 10s

---

## Scheduler Job Matrix

| Job | Schedule | Lock Key | TTL | Description |
|---|---|---|---|---|
| Scrape today | On startup + every 1 minute | `scheduler:lock:scrape:today` | 10 min | Scrapes events happening today |
| Scrape future | On startup + 06:00, 18:00 UTC | `scheduler:lock:scrape:future` | 30 min | Scrapes events for next 7 days |
| Daily stats | 00:01 UTC daily | `scheduler:lock:stats:daily` | 10 min | Generates daily content stats |
| Monthly stats | 00:10 on 1st of month | `scheduler:lock:stats:monthly` | 30 min | Generates monthly content stats |
| Upload cleanup | Every 15 minutes | `scheduler:lock:uploads:cleanup` | 10 min | Cleans stale uploads and orphans |
| Download counter flush | Every 15 minutes | `scheduler:lock:apk-downloads:flush` | 10 min | Flushes Redis download counts to DB |

All jobs use distributed Redis locks to prevent duplicate execution across instances. If a lock cannot be acquired, the job is silently skipped.

---

## Native Production Deployment

Merges to `main` deploy automatically after the `CI` workflow succeeds. The deploy workflow runs only on the self-hosted runner labeled `iptv`, installs the exact SHA reported by the successful CI run, and refuses publication if that SHA is no longer the current remote `main`.

Production layout:

| Path | Purpose |
|---|---|
| `/opt/iptv/iptv` | Native Go executable |
| `/opt/iptv/web/dist` | Vue dashboard assets |
| `/opt/iptv/apk_storage` | Persistent APK data; never replaced by deployment |
| `/opt/iptv/image_storage` | Persistent team logos; never replaced by deployment |
| `/opt/iptv/.deploy/previous` | Previous executable and dashboard for artifact rollback |

The existing user service is `/home/iptv/.config/systemd/user/iptv.service`. Its environment remains outside the repository. Startup applies forward database migrations; automatic artifact rollback does not run down migrations.

Verification and diagnostics:

```bash
systemctl --user status iptv.service --no-pager
curl --fail http://127.0.0.1:8080/health/ready
journalctl --user -u iptv.service -n 100 --no-pager
```

Dashboard publication and rollback atomically exchange same-filesystem directories, so `/opt/iptv/web/dist` remains present throughout each operation. If health verification fails, the deployment script restores the previous binary and dashboard and restarts the service. If a migration prevents application rollback, follow `docs/operations/rollback.md` instead of manually running down migrations.

## Common Operational Commands

```bash
# Development
docker compose -f deployments/docker/compose.dev.yml up --build

# Native production service
systemctl --user restart iptv.service
systemctl --user status iptv.service --no-pager
curl --fail http://127.0.0.1:8080/health/ready

# Run tests (requires compose.test.yml services up)
docker compose -f deployments/docker/compose.test.yml up -d mariadb redis
docker compose -f deployments/docker/compose.test.yml run backend-init
docker compose -f deployments/docker/compose.test.yml up backend

# Bootstrap invitation token (first-time setup)
go run ./cmd/server bootstrap-invitation

# Run Go tests with race detection
go test -race ./...

# Run Go tests with coverage
go test -coverprofile=coverage.out ./...

# Frontend
cd web && npm run dev
cd web && npm run build
cd web && npm run lint
cd web && npm run test:unit
cd web && npm run test:e2e

# Database migration (happens automatically on startup)
# Direct DSN for manual operations:
# mysql://root:password@host:3306/sofascore

# Prometheus metrics
curl http://localhost:8080/metrics

# Pprof (if PPROF_ADDR is set)
go tool pprof http://localhost:6060/debug/pprof/heap
```

---

## Prometheus Metrics

Metrics available at `/metrics`:
- `sofascore_http_requests_total` — HTTP request count by method, path, status
- `sofascore_http_request_duration_seconds` — HTTP request duration histogram
- `sofascore_db_connections` — Database connection pool gauge (open, idle, in_use)
- `sofascore_db_connection_wait_seconds` — Database connection wait duration
- `sofascore_redis_connections` — Redis connection pool gauge
- `sofascore_redis_command_duration_seconds` — Redis command duration histogram

## Architectural Overview

```
┌──────────────────────────────────────────────────────────┐
│                      nginx (:80)                         │
│                  (compose.multi only)                    │
└────────────┬──────────────┬──────────────┬──────────────┘
     ┌───────┴──────┐ ┌─────┴──────┐ ┌─────┴──────┐
     │  backend-1   │ │ backend-2  │ │ backend-3  │
     │   :8080      │ │  :8080     │ │  :8080     │
     └───┬────┬─────┘ └──┬────┬────┘ └──┬────┬────┘
         │    │          │    │         │    │
    ┌────┴┐   │     ┌────┴┐   │    ┌────┴┐   │
    │ DB  │   │     │ DB  │   │    │ DB  │   │
    └─────┘   │     └─────┘   │    └─────┘   │
         │    │          │    │         │    │
         └────┼──────────┼────┼─────────┼────┘
              │          │    │         │
         ┌────┴──────────┴────┴─────────┴────┐
         │          MariaDB 10.11             │
         └───────────────────────────────────┘
              │          │    │         │
         ┌────┴──────────┴────┴─────────┴────┐
         │         Redis 7 (Alpine)           │
         └───────────────────────────────────┘

All backends share one MariaDB and one Redis instance.
Each backend runs identical scheduler jobs gated by Redis distributed locks.
The SPA frontend is served directly by each backend from web/dist/.
```
