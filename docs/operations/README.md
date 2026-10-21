# Operations

Entry point for **operators, sysadmins, and SREs** running the
scores365-scrapper platform.

## Two deployment paths

| Path | Use when | Doc |
|---|---|---|
| **Docker Compose for testing** | Local dev, CI, load testing, pre-prod | See [Docker Compose for testing](#docker-compose-for-testing) below |
| **Ubuntu `.deb` for production** | Production on Ubuntu 22.04/24.04 amd64 | [`deb-package.md`](deb-package.md) |
| **Production-like Docker chain test** | Validate a single `.deb` build locally before pushing | [`production-like-local-deploy.md`](production-like-local-deploy.md) |

Production deploys go through the `.deb` package only. Native binary
deploys are not supported.

## Docker Compose for testing

Three compose files live in `deployments/docker/`:

| File | Purpose | Notes |
|---|---|---|
| `compose.dev.yml` | Local development | Persistent volumes; one backend |
| `compose.test.yml` | CI / integration tests | tmpfs; one backend + bootstrap-invitation one-shot |
| `compose.multi.yml` | Staging / load testing | Three backends behind nginx + Prometheus |

The env file you need is `deployments/docker/.env.example`, **not** at
the repo root. Local dev workflow:

```bash
cp deployments/docker/.env.example .env       # set DB_PASSWORD and JWT_SECRET
docker compose -f deployments/docker/compose.dev.yml up --build
curl http://localhost:8080/health/live
```

Chain test (installs every `.deb` in `dist/` in order; the last one
must come up healthy):

```bash
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .
docker run --rm -p 8080:8080 -v "$PWD/dist:/tmp/dist:ro" iptv-chain-test
```

## Environment variables

Single source of truth. The `.deb` postinst writes these to
`/etc/iptv/env` on first install; the compose files read them from
`.env` and the shell environment.

Every entry below comes from `internal/config/config.go:Load`. When
the value is empty (no default), the variable is required.

### Server

| Variable | Default | Required | Description |
|---|---|---|---|
| `API_ADDR` | `:8080` | No | HTTP listen address |
| `JWT_SECRET` | *(none)* | **Yes** | JWT signing secret (≥ 32 chars; see `MinJWTSecretLength` in `internal/config/config.go`). |
| `SKIP_MIGRATE` | `false` | No | When `true`, skips GORM AutoMigrate and the default-admin seeder on startup. Use when you manage schema externally. |
| `APK_STORAGE_PATH` | `./apk_storage` | No | APK chunk storage directory |
| `IMAGE_STORAGE_PATH` | `./image_storage` | No | Team logo storage directory |
| `SCRAPE_BATCH_SIZE` | `500` | No | Events per scrape batch |
| `SCRAPE_CONCURRENCY` | `8` | No | Concurrent scrape goroutines |
| `PUSH_TIMER_TICK_INTERVAL` | `5s` | No | How often the push-timer runner wakes to dispatch due scheduled pushes. Smaller = lower perceived delay, more queries. |
| `PUSH_TIMER_BATCH_LIMIT` | `100` | No | Max timers the runner processes per tick. Raise when campaigns skew to large audiences. |

### Observability / pprof

| Variable | Default | Required | Description |
|---|---|---|---|
| `PPROF_ADDR` | *(empty)* | No | pprof listen address (e.g. `:6060`). |
| `ENABLE_PPROF` | `false` | No | When `true` **and** `PPROF_ADDR` is set, binds the pprof HTTP endpoint. pprof never binds if either is unset. Treat as a debug-only feature. |

### Database (MariaDB)

| Variable | Default | Required | Description |
|---|---|---|---|
| `DB_HOST` | `localhost` | Yes | MariaDB hostname |
| `DB_PORT` | `3306` | No | MariaDB port |
| `DB_USER` | `root` | No | MariaDB user |
| `DB_PASSWORD` | *(empty)* | Yes (prod) | MariaDB password |
| `DB_NAME` | `scores365` | No | Database name |
| `DB_MAX_OPEN_CONNS` | `25` | No | Max open DB connections |
| `DB_MAX_IDLE_CONNS` | `10` | No | Max idle DB connections |
| `DB_CONN_MAX_LIFETIME` | `30m` | No | Max connection lifetime |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | No | Max connection idle time |

### Redis

| Variable | Default | Required | Description |
|---|---|---|---|
| `REDIS_URL` | `redis://localhost:6379/0` | No | Redis connection URL |
| `REDIS_KEY_PREFIX` | *(empty)* | No | Key prefix when sharing a Redis instance across deployments. Set to a unique value (e.g. `prod-`) on each stack to stop cross-stack key collisions. |
| `REDIS_DIAL_TIMEOUT` | `5s` | No | Redis dial timeout |
| `REDIS_READ_TIMEOUT` | `3s` | No | Redis read timeout |
| `REDIS_WRITE_TIMEOUT` | `3s` | No | Redis write timeout |

### HTTP server timeouts and proxies

| Variable | Default | Required | Description |
|---|---|---|---|
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | No | HTTP read-header timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | No | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `120s` | No | HTTP idle timeout |
| `TRUSTED_PROXIES` | *(empty)* | No | Comma-separated list of trusted proxy IPs / CIDRs. Empty = Gin's defaults; populate when fronted by nginx so client-IP and rate-limit logic survive the hop. |

### Scraping

| Variable | Default | Required | Description |
|---|---|---|---|
| `SCORES365_LANG_ID` | `29` | No | 365scores language id; `29` is es-mx. Changes the language of league, team and status text. |
| `SCORES365_TIMEZONE` | `UTC` | No | IANA zone the 365scores feed buckets a day of fixtures into. Leave at `UTC` unless a day boundary needs to follow a local timezone. |
| `SCORES365_COUNTRY_ID` | `116` | No | 365scores user-country id. Only affects the popularity ordering of the competitions catalog, not which competitions are returned. |
| `SCORES365_DISCOVERY_SPORTS` | *(built-in list)* | No | Comma-separated sport slugs the catalog discovery sweep enrols, e.g. `basketball,tennis,baseball`. Empty uses the built-in list (8 sports except football). Unknown slugs are logged and skipped. |
| `FOTMOB_TIMEZONE` | `Europe/Paris` | No | IANA zone passed to FotMob's `/api/data/matches` so the upstream's day-bucketing matches its editorial day. Keep aligned with `SCORES365_TIMEZONE`; see [Troubleshooting → Timezone drift](troubleshooting.md#detection-catalog--known-production-failure-modes). |

> **Football is intentionally omitted from `SCORES365_DISCOVERY_SPORTS`'s
> default list.** FotMob remains the primary source for football;
> turning football on here would enrol ~1.5k football competitions
> that the dispatcher would then route away from FotMob
> (`internal/scraper/catalog/discovery.go:53`).

## Pre-deployment checklist (Ubuntu `.deb` production)

- [ ] Outbound HTTPS to `webws.365scores.com`, `imagecache.365scores.com`,
      `www.fotmob.com` and `images.fotmob.com` is reachable from the
      server.
- [ ] Inbound TCP `80` (and/or `8080` if the dashboard is exposed
      directly) is allowed by the firewall.
- [ ] DNS records for the dashboard hostname and for the APK
      distribution domain point to the server's public IP.
- [ ] The `iptv` system user does not yet exist.
- [ ] `mariadb-server` (≥ 10.5) and `redis-server` (≥ 5.0) are
      installable from the configured apt sources.
- [ ] A strong `JWT_SECRET` (≥ 32 chars) is decided for
      `/etc/iptv/env`.
- [ ] A key prefix is set when sharing Redis with anything else
      (`REDIS_KEY_PREFIX=prod-`).
- [ ] Backup destination configured (see [`rollback.md`](rollback.md)).

After install:

- [ ] `curl http://127.0.0.1:8080/health/ready` returns `200`.
- [ ] `curl http://127.0.0.1:8080/health/live` returns `200`.
- [ ] `systemctl status iptv.service` shows `active (running)`.
- [ ] `/etc/iptv/env` exists with `0640 root:iptv` permissions and a
      non-empty `JWT_SECRET`.
- [ ] `sudo mariadb -uroot -e "SHOW DATABASES"` lists `iptv`.

## Service management

```bash
sudo systemctl status iptv.service
sudo systemctl restart iptv.service
sudo journalctl -u iptv.service -f
```

## Working with branches

Use git worktrees inside the project, under `.worktrees/` at the
repo root:

```bash
git worktree add .worktrees/<branch> -b <branch>       # new branch
git worktree remove .worktrees/<branch>                 # cleanup
```

`.worktrees/` is gitignored. When the branch is merged or
abandoned, remove the worktree with `git worktree remove` rather
than deleting the folder manually.

## Where to escalate

- Day-to-day operations (alerts, Redis outage, counter recovery,
  cache invalidation, scraper catalog) —
  [`runbook.md`](runbook.md).
- Symptom-first failure catalog and recovery —
  [`troubleshooting.md`](troubleshooting.md).
- Verifying that events are actually reaching the database, and by
  which source — [`verify-scraper-output.md`](verify-scraper-output.md).
- Rollback procedures — [`rollback.md`](rollback.md).
- Upstream dependencies —
  [365scores](https://www.365scores.com/es-mx),
  [FotMob](https://www.fotmob.com),
  [MariaDB](https://mariadb.org),
  [Redis](https://redis.io).
