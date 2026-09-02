# Operations

Entry point for **operators, sysadmins, and SREs** running the
sofascore-scrapper platform.

## Two deployment paths

| Path | Use when | Doc |
|---|---|
| **Docker Compose for testing** | Local dev, CI, load testing, pre-prod | See [Docker Compose](#docker-compose-for-testing) below |
| **Ubuntu `.deb` for production** | Production on Ubuntu 22.04/24.04 amd64 | [`deb-package.md`](deb-package.md) |
| **Production-like Docker (.deb on Ubuntu 24.04)** | Validate a single `.deb` build locally before pushing | [`production-like-local-deploy.md`](production-like-local-deploy.md) |

Production deploys go through the `.deb` package only. Native binary
deploys are not supported.

## Docker Compose for testing

Three compose files live in `deployments/docker/`:

| File | Purpose | Notes |
|---|---|---|
| `compose.dev.yml` | Local development | Persistent volumes; one backend |
| `compose.test.yml` | CI / integration tests | tmpfs; one backend + bootstrap-invitation one-shot |
| `compose.multi.yml` | Staging / load testing | Three backends behind nginx + Prometheus |

Local dev workflow:

```bash
cp .env.example .env                  # set DB_PASSWORD and JWT_SECRET
docker compose -f deployments/docker/compose.dev.yml up --build
curl http://localhost:8080/health/live
```

Chain test (install every `.deb` in `dist/` in order; the last one
must come up healthy):

```bash
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .
docker run --rm -p 8080:8080 -v "$PWD/dist:/tmp/dist:ro" iptv-chain-test
```

## Environment variables

Single source of truth. The `.deb` postinst writes these to
`/etc/iptv/env` on first install; the compose files read them from
`.env` and shell environment.

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
| `REDIS_KEY_PREFIX` | *(empty)* | No | Redis key prefix (use when sharing an instance) |
| `REDIS_DIAL_TIMEOUT` | `5s` | No | Redis dial timeout |
| `REDIS_READ_TIMEOUT` | `3s` | No | Redis read timeout |
| `REDIS_WRITE_TIMEOUT` | `3s` | No | Redis write timeout |
| `JWT_SECRET` | *(none)* | **Yes** | JWT signing secret (≥ 32 chars) |
| `API_ADDR` | `:8080` | No | HTTP listen address |
| `PPROF_ADDR` | *(empty)* | No | pprof debug address (e.g. `:6060`) |
| `APK_STORAGE_PATH` | `./apk_storage` | No | APK chunk storage directory |
| `IMAGE_STORAGE_PATH` | `./image_storage` | No | Team logo storage directory |
| `SCRAPE_BATCH_SIZE` | `500` | No | Events per scrape batch |
| `SCRAPE_CONCURRENCY` | `8` | No | Concurrent scrape goroutines |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | No | HTTP read header timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | No | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `120s` | No | HTTP idle timeout |

## Pre-deployment checklist (Ubuntu `.deb` production)

- [ ] Outbound HTTPS to `www.sofascore.com` is reachable from the server.
- [ ] Inbound TCP `80` (and/or `8080` if the dashboard is exposed directly) is allowed by the firewall.
- [ ] DNS records for the dashboard hostname and for the APK distribution domain point to the server's public IP.
- [ ] The `iptv` system user does not yet exist.
- [ ] `mariadb-server` (≥ 10.5) and `redis-server` (≥ 5.0) are installable from the configured apt sources.
- [ ] A strong `JWT_SECRET` is decided for `/etc/iptv/env`.
- [ ] Backup destination configured (see [`rollback.md`](rollback.md)).

After install:

- [ ] `curl http://127.0.0.1:8080/health/ready` returns `200`.
- [ ] `curl http://127.0.0.1:8080/health/live` returns `200`.
- [ ] `systemctl status iptv.service` shows `active (running)`.
- [ ] `/etc/iptv/env` exists with `0640 root:iptv` permissions and a non-empty `JWT_SECRET`.
- [ ] `sudo mariadb -uroot -e "SHOW DATABASES"` lists `iptv`.

## Service management

```bash
sudo systemctl status iptv.service
sudo systemctl restart iptv.service
sudo journalctl -u iptv.service -f
```

## Where to escalate

- Day-to-day operations (alerts, Redis outage, counter recovery, cache invalidation) — [`runbook.md`](runbook.md).
- Rollback procedures — [`rollback.md`](rollback.md).
- Upstream dependencies — [SofaScore](https://www.sofascore.com), [MariaDB](https://mariadb.org), [Redis](https://redis.io).