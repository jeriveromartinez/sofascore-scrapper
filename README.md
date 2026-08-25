# sofascore-scrapper

IPTV backend service — scrapes sports events from Sofascore, manages APK distribution, device registration, playback tracking, and provides a web dashboard.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     nginx (:80)                          │
│                 (multi-instance only)                    │
└────────────┬──────────────┬──────────────┬──────────────┘
     ┌───────┴──────┐ ┌─────┴──────┐ ┌─────┴──────┐
     │  backend-1   │ │ backend-2  │ │ backend-3  │
     │   :8080      │ │  :8080     │ │  :8080     │
     └───┬────┬─────┘ └──┬────┬────┘ └──┬────┬────┘
         │    │          │    │         │    │
         └────┼──────────┼────┼─────────┼────┘
              └──────────┼────┼─────────┘
                    ┌────┴────┴────┐
                    │ MariaDB 10.11│
                    └──────┬───────┘
                    ┌──────┴───────┐
                    │  Redis 7     │
                    └──────────────┘
```

All backends share one MariaDB and one Redis instance. Scheduled jobs (scrape, stats, upload cleanup, download counter flush) use Redis distributed locks to prevent duplicate execution.

## Stack

- **Backend:** Go 1.25, Gin, GORM (MariaDB), go-redis, golang-migrate, robfig/cron
- **Frontend:** Vue 3, Vite, Pinia, TypeScript, Playwright (E2E), Vitest (unit)
- **Infrastructure:** Docker (multi-stage), nginx, Prometheus

## Project Structure

```
.
├── cmd/server/              # Application entry point
├── internal/
│   ├── apk/                 # APK upload, download, versioning
│   ├── app/                 # Application bootstrap, router, shutdown
│   ├── auth/                # JWT, refresh tokens, invitations
│   ├── config/              # Environment configuration
│   ├── devices/             # Device registration and management
│   ├── domains/             # Domain whitelist management
│   ├── events/              # Scraped event storage, caching, logos
│   ├── gen/                 # Generated protobuf code
│   ├── pagination/          # Cursor pagination utilities
│   ├── platform/
│   │   ├── database/        # GORM connection, auto-migration
│   │   ├── observability/   # Structured logging, Prometheus, pprof
│   │   └── redis/           # Redis client, distributed locks, rate limit
│   ├── playback/            # Playback tracking
│   ├── reporting/           # Crash reports, content stats, aggregation
│   ├── scheduler/           # Distributed job scheduler (cron + locks)
│   ├── scraper/             # Sofascore HTML scraping via chromedp
│   ├── server/              # Gin router, middleware, health, metrics, dashboard
│   ├── testing/             # Test utilities
│   ├── tournaments/         # Tournament, device assignments, global config
│   └── users/               # User management
├── migrations/              # Embedded SQL migrations (golang-migrate)
├── web/                     # Vue 3 SPA dashboard
├── deployments/docker/      # Docker Compose files for dev, test, multi
├── proto/                   # Protobuf API definitions
├── docs/
│   ├── operations/          # Runbook and rollback procedures
│   └── performance/         # Load test and performance documentation
└── tests/                   # Integration and E2E tests
```

## Environment Variables

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
| `REDIS_KEY_PREFIX` | *(empty)* | No | Redis key prefix |
| `REDIS_DIAL_TIMEOUT` | `5s` | No | Redis dial timeout |
| `REDIS_READ_TIMEOUT` | `3s` | No | Redis read timeout |
| `REDIS_WRITE_TIMEOUT` | `3s` | No | Redis write timeout |
| `JWT_SECRET` | *(none)* | **Yes** | JWT signing secret |
| `API_ADDR` | `:8080` | No | HTTP listen address |
| `PPROF_ADDR` | *(empty)* | No | Pprof debug address (e.g. `:6060`) |
| `APK_STORAGE_PATH` | `./apk_storage` | No | APK chunk storage directory |
| `IMAGE_STORAGE_PATH` | `./image_storage` | No | Team logo storage directory |
| `SCRAPE_BATCH_SIZE` | `500` | No | Events per scrape batch |
| `SCRAPE_CONCURRENCY` | `8` | No | Concurrent scrape goroutines |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | No | HTTP read header timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | No | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `120s` | No | HTTP idle timeout |

## Quick Start

### Docker Compose (development)

```bash
docker compose -f deployments/docker/compose.dev.yml up --build
```

Requires `.env` with `JWT_SECRET` set.

### Docker Compose (multi-instance with nginx + Prometheus)

```bash
docker compose -f deployments/docker/compose.multi.yml up --build
```

### Local Development

Requirements: Go 1.25+, MariaDB, Redis, Node.js 22+.

```bash
# Backend
export DB_HOST=localhost
export DB_PASSWORD=yourpassword
export JWT_SECRET=your-secret-at-least-32-chars
go run ./cmd/server

# Bootstrap invitation token (first user registration)
go run ./cmd/server bootstrap-invitation

# Frontend
cd web
npm ci
npm run dev
```

## Commands

```bash
# Go tests
go test -race ./...

# Go vet
go vet ./...

# Frontend
cd web && npm run lint
cd web && npm run test:unit
cd web && npm run test:e2e
cd web && npm run build

# Regenerate Go protobuf bindings (this repo is the source of truth for proto/api.proto)
make proto

# Verify generated bindings match proto/api.proto
make proto-check

# Verify flutter-apptv/proto/api.proto mirrors this repo's (cross-repo contract)
make proto-verify-flutter

# Generate protobuf (Vue/TypeScript) — see web/package.json scripts
cd web && npm run proto
```

> **Proto sync contract.** `proto/api.proto` in this repo is the single source of
> truth. The Flutter client (`flutter-apptv/proto/api.proto`) and the Vue/TypeScript
> web client (`web/src/...`) must mirror it. CI enforces the Go bindings via
> `make proto-check`; the cross-repo check against Flutter is `make proto-verify-flutter`
> (run it locally before opening a PR that touches `proto/api.proto`).

## API Endpoints

**Auth column:** `None` public · `Token` download-token capability · `Device` registered-device header (`APP-XIPTV`) · `Auth` JWT bearer (any user) · `Admin` JWT bearer with the `admin` role.

### Infrastructure

| Path | Auth | Description |
|---|---|---|
| `GET /` | None | SPA dashboard |
| `GET /health/live` | None | Liveness probe |
| `GET /health/ready` | None | Readiness probe (DB + Redis) |
| `GET /metrics` | None | Prometheus metrics (blocked at the edge in multi-instance; see `nginx.conf`) |

### App API (`/api/app/v1`) — consumed by Android devices

| Method / Path | Auth | Description |
|---|---|---|
| `GET /update` | None | Latest APK version / update check |
| `GET /apk/download/:token` | Token | Download APK by capability token |
| `GET /devices/url/:packageName` | None | Resolve distribution domain for a package |
| `POST /devices` | None | Register device |
| `POST /devices/viewing` | Device | Report playback / viewing |
| `GET /current-events` | Device | Current + upcoming events for the device |
| `POST /crash-report` | None | Submit crash report (IP rate-limited, body-size capped) |
| `GET /teams/logo/:teamId` | None | Team logo image |

### Management API (`/api/web/v1`) — dashboard / operators

| Method / Path | Auth | Description |
|---|---|---|
| `POST /users/register` | None | Register (requires a valid invitation token) |
| `POST /users/login` | None | Obtain access + refresh tokens |
| `POST /users/refresh` | None | Rotate access + refresh tokens |
| `POST /users/logout` | Auth | Revoke refresh token(s) |
| `POST /users/invitations` | Admin | Create an invitation token |
| `PUT /users/:id/role` | Admin | Set a user's role (`admin`/`user`); refuses to demote the last admin |

Every other `/api/web/v1/*` route requires **Admin** — user, device, event, playback, APK
(including resumable uploads), tournament, domain, and global-config management, plus
reporting (`GET /stats/top-events`). See `internal/app/routes.go` for the authoritative list.

## Authorization & Roles

The management API uses JWT bearer authentication (`AuthMiddleware`) plus role-based access control:

- **Roles:** `admin` and `user` (`users.role`, default `user`).
- **Admin gate:** `RequireAdmin` runs after authentication on every `/api/web/v1/*` management route and on invitation creation; non-admins receive `403`.
- **Middleware order:** authenticate → rate-limit (fail-closed) → admin check, so an unauthenticated flood is throttled before any database lookup.
- **Bootstrapping:** migration `000008_user_roles` adds the column and promotes every pre-existing account to `admin` (they were implicit operators). New invited accounts default to `user`.
- **Promoting / demoting:** `PUT /api/web/v1/users/:id/role` (admin only); the last remaining admin cannot be demoted. Inspect roles directly with `SELECT email, role FROM users;`.

## Data Model

Core tables (see `migrations/000001_baseline.up.sql` for full DDL):

| Table | Purpose |
|---|---|
| `users` | Operator accounts (email, bcrypt password, `role`: admin/user) |
| `events` | Scraped sport events with scores, timestamps |
| `teams` | Team metadata and logos |
| `tournaments` | Tournament/league registry |
| `devices` | Registered Android devices |
| `playback_logs` | Playback start/end records |
| `apk_versions` | APK binary metadata and versions |
| `content_stats` | Aggregated view statistics |
| `crash_reports` | Android crash report storage |
| `domains` | DNS domain whitelist |
| `device_tournaments` | Device-to-tournament assignments |
| `global_tournament_configs` | Global tournament visibility |
| `refresh_tokens` | JWT refresh token store |
| `download_counter_flushes` | Counter flush idempotency |
| `apk_upload_publications` | Chunked upload state |

## Scheduled Jobs

| Job | Schedule | Lock TTL |
|---|---|---|
| Scrape today | Every 1 minute | 10 min |
| Scrape future | 06:00 and 18:00 UTC | 30 min |
| Daily content stats | 00:01 UTC | 10 min |
| Monthly content stats | 00:10 on 1st | 30 min |
| Upload cleanup | Every 15 minutes | 10 min |
| Download counter flush | Every 15 minutes | 10 min |

## Operations

See [docs/operations/runbook.md](docs/operations/runbook.md) for production runbook (alerts, Redis outage behavior, counter recovery, cache invalidation).

See [docs/operations/rollback.md](docs/operations/rollback.md) for rollback procedures (migration policy, backup/restore, destructive migration warnings).
