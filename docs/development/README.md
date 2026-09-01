# Development Guide

Entry point for engineers writing, building, and testing the
sofascore-scrapper backend, the Vue admin dashboard, and the
packaging pipeline.

For installation, deployment, and day-to-day operations, see
[`../operations/README.md`](../operations/README.md). For the product
overview, see [`../sales/README.md`](../sales/README.md).

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

All backends share one MariaDB and one Redis. Scheduled jobs use
Redis distributed locks so duplicates do not fire when more than one
backend is running.

## Stack

- **Backend:** Go 1.25, Gin, GORM (MariaDB), go-redis, golang-migrate, robfig/cron
- **Frontend:** Vue 3, Vite, Pinia, TypeScript, Playwright (E2E), Vitest (unit)
- **Wire format:** Protocol Buffers v3; single source of truth in [`proto/api.proto`](../../proto/api.proto)

## Project layout

```
cmd/server/                # main entry point
internal/                  # Go packages (one per domain)
  auth/                    # login / register / token refresh
  apk/                     # APK upload + chunked upload + download counter
  config/                  # env loading
  devices/                 # device registration
  domains/                 # APK distribution domains
  events/                  # scraped events + admin query
  pagination/              # cursor encoding
  playback/                # viewing reports
  platform/database        # GORM setup
  platform/observability   # logging / metrics
  platform/redis           # Redis client + locker
  push/                    # push notifications (immediate + scheduled)
  realtime/                # WebSocket hub
  reporting/               # crash reports + stats
  scheduler/               # cron-driven jobs
  scraper/                 # SofaScore HTTP client + batch upsert
  server/                  # HTTP server bootstrap
  testing/                 # test fixtures
  tournaments/             # tournament CRUD + device assignments
  users/                   # user CRUD + roles
migrations/                # SQL migrations (numbered pairs)
proto/api.proto            # wire format source of truth
web/                       # Vue admin dashboard
deployments/               # Docker compose + .deb packaging
docs/                      # documentation
scripts/                   # Operator / dev smoke scripts (auth_smoke, …)
tests/                     # Integration and load tests (k6, Go integration)
```

## Building

### Go binary

```bash
go build -o ./bin/sofascore-scrapper ./cmd/server
```

For a release build with buildinfo overrides:

```bash
VERSION=v0.1.0 COMMIT=$(git rev-parse --short HEAD) \
    go build -trimpath -ldflags="-s -w \
        -X github.com/jeriveromartinez/sofascore-scrapper/internal/buildinfo.Version=$VERSION \
        -X github.com/jeriveromartinez/sofascore-scrapper/internal/buildinfo.Commit=$COMMIT" \
        -o ./bin/sofascore-scrapper ./cmd/server
```

### Vue dashboard

```bash
cd web
npm ci
npm run build
```

Output lands in `web/dist/`. The backend embeds it at compile time.

### Ubuntu `.deb` package

The build runs entirely inside Docker — no host Go/Node toolchain
required. See [`../operations/deb-package.md`](../operations/deb-package.md)
for the full procedure.

```bash
./deployments/package/build-deb.sh [version]
# → dist/iptv_<version>_amd64.deb
```

## Testing

### Unit + integration

```bash
go test ./...
```

Tests are colocated with the package they cover (`internal/foo/foo_test.go`).
The integration-tagged tests are gated by the `integration` build tag.

### Chain test (Docker)

The chain test installs every `.deb` in `dist/` in order and asserts
that the **last** one (the current proposal) comes up healthy. It
catches migration regressions and packaging breakages.

```bash
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .
docker run --rm -p 8080:8080 -v "$PWD/dist:/tmp/dist:ro" iptv-chain-test
```

### Docker Compose for local dev

```bash
cp .env.example .env
docker compose -f deployments/docker/compose.dev.yml up --build
```

The same compose file is used for the dev loop and for the staging
stack (multi-instance via `compose.multi.yml`).

### Auth round-trip smoke test

`scripts/auth_smoke/` is a tiny Go program that exercises every
`/api/web/v1/users/*` endpoint with the real protobuf wire format. It
needs a running backend (any of the three Docker setups above, or a
natively-run `go run ./cmd/server`) and a bootstrap invitation token you
paste into the source file:

```bash
# 1. Get the token
go run ./cmd/server bootstrap-invitation           # native
# or
docker exec iptv-prod sudo -u iptv /opt/iptv/iptv bootstrap-invitation # .deb

# 2. Paste the token into scripts/auth_smoke/main.go

# 3. Run the round-trip
go run ./scripts/auth_smoke/
```

The script prints each step's status (register 201, get-users 200,
invitation 201, refresh 200, logout 200) and is the recommended way to
verify the auth stack works end-to-end. It uses a unique
`admin-<unix-ts>@test.local` email so re-runs don't collide with
already-registered users.

For the full production-like deploy (Ubuntu 24.04 + real `.deb` +
MariaDB + Redis + systemd-style env), see
[`../operations/production-like-local-deploy.md`](../operations/production-like-local-deploy.md).

### Auth wire format

Every `/api/web/v1/users/*` endpoint accepts and returns **protobuf
binary**, not JSON. The `Content-Type` for both request and response is
`application/x-protobuf`. The Vue dashboard and the Android client use
the generated bindings (`web/src/api/`, `flutter-apptv/lib/api/`) and
get this for free; if you `curl` these endpoints, you need to encode the
request with `google.golang.org/protobuf/proto.Marshal` and decode the
response with `proto.Unmarshal`. Sending JSON results in
`{"error":"email and password are required"}` because `proto.Unmarshal`
silently ignores the JSON bytes and the handler sees zero-valued fields.

`/api/app/v1/crash-report` is the exception — it accepts JSON because
clients are crash reporters on Android, not generated bindings.

### Load test

See [`../performance/load-test.md`](../performance/load-test.md) for the
k6 setup, scenarios, and thresholds.

## Protobuf contract

The wire format is defined in [`proto/api.proto`](../../proto/api.proto).
Regenerate Go bindings after editing:

```bash
make proto
```

The Flutter consumer (the Android TV app) mirrors `proto/api.proto`
verbatim. Verify the mirror is in sync before tagging a release:

```bash
make proto-verify-flutter
```

If the Flutter proto is out of sync, copy the Go file into the
Flutter repo and regenerate the Dart bindings there.

## Environment variables

See [`../operations/README.md`](../operations/README.md#environment-variables)
for the single source of truth. Local development typically only needs
`DB_PASSWORD` and `JWT_SECRET` in `.env`; the rest have safe defaults.

## Conventions

- **Commit messages** — Conventional Commits in English, e.g.
  `feat(events): add cursor pagination to admin events query`.
- **Branch names** — `<type>/<short-kebab-description>`, e.g.
  `feat/scraper-403-cookie-refresh`.
- **PRs** — mergeable only when `ci.yml`, `integration.yml`, and
  `e2e.yml` are green. PR body must include what changed, why, and
  how it was tested.

## Where to escalate

- Architecture questions — open an issue tagged `architecture`.
- Operational concerns (anything that touches `deployments/` or the
  `.deb` package) — flag in the PR; the on-call operator will review.
- Security — see [`../../SECURITY.md`](../../SECURITY.md).
