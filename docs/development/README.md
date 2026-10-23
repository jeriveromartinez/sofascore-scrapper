# Development Guide

Entry point for engineers writing, building, and testing the
scores365-scrapper backend, the Vue admin dashboard, and the
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

All backends share one MariaDB and one Redis. Scheduled jobs use Redis
distributed locks so duplicates do not fire when more than one backend
is running.

## Stack

- **Backend** — Go 1.25, Gin, GORM (MariaDB), go-redis, robfig/cron
- **Frontend** — Vue 3, Vite, Pinia, TypeScript, Playwright (E2E), Vitest (unit)
- **Wire format** — Protocol Buffers v3; single source of truth in
  [`proto/api.proto`](../../proto/api.proto)

## Project layout

```
cmd/server/                       # main entry point
internal/                         # Go packages (one per domain)
  app/                            # router wiring + application lifecycle
  apk/                            # APK upload + chunked upload + download counter
  auth/                           # login / register / refresh / token service
  buildinfo/                      # /api/web/v1/version endpoint (ldflags-injected)
  config/                         # env loading
  devices/                        # device registration
  domains/                        # APK distribution domains
  events/                         # scraped events + current-events cache + epoch
  gen/                            # protobuf-generated Go bindings (api.pb.go)
  pagination/                     # cursor encoding helpers
  playback/                       # viewing reports
  platform/database/              # GORM setup + AutoMigrateAll
  platform/observability/         # logging / metrics
  platform/redis/                 # Redis client + locker
  push/                           # push notifications (immediate + scheduled)
  realtime/                       # WebSocket hub
  reporting/                      # crash reports + stats aggregations
  scheduler/                      # cron-driven jobs (scraper, reporting)
  scores365/                      # 365scores HTTP client
  scraper/                        # shared service (dispatch over sources)
  scraper/catalog/                # league catalog (CRUD + discovery)
  scraper/fotmob/                 # FotMob source adapter
  scraper/scores365/              # 365scores source adapter
  seeder/                         # default admin + curated scraper-leagues seed
  server/                         # HTTP server bootstrap (middleware, health, rate limit)
  testing/                        # shared test fixtures
  tournaments/                    # tournament CRUD + device assignments + global config
  users/                          # user CRUD + roles
proto/api.proto                   # wire format source of truth
web/                              # Vue admin dashboard
deployments/                      # Docker compose + .deb packaging
docs/                             # documentation
scripts/                          # dev / operator smoke scripts
tests/                            # k6 load tests, Go integration tests
```

## Building

### Go binary

```bash
go build -o ./bin/scores365-scrapper ./cmd/server
```

For a release build with buildinfo overrides:

```bash
VERSION=v0.1.0 COMMIT=$(git rev-parse --short HEAD) \
    go build -trimpath -ldflags="-s -w \
        -X github.com/jeriveromartinez/scores365-scrapper/internal/buildinfo.Version=$VERSION \
        -X github.com/jeriveromartinez/scores365-scrapper/internal/buildinfo.Commit=$COMMIT" \
        -o ./bin/scores365-scrapper ./cmd/server
```

### Vue dashboard

```bash
cd web
npm ci
npm run build
```

Output lands in `web/dist/`. The Docker build's `build-vue` stage copies
it into the image as `web/dist`; the backend serves those files from
disk (there is no `go:embed`), relative to its working directory.

### Ubuntu `.deb` package

The build runs entirely inside Docker — no host Go/Node toolchain
required. See [`../operations/deb-package.md`](../operations/deb-package.md)
for the full procedure.

```bash
./deployments/package/build-deb.sh [version]
# → dist/iptv_<version>_amd64.deb
```

## Testing

### Unit tests

```bash
go test ./...
```

Tests are colocated with the package they cover
(`internal/foo/foo_test.go`).

### Integration tests

Integration tests are gated by the `integration` build tag. Run them
locally with a running MariaDB and Redis (the `.github/workflows/integration.yml`
workflow shows the required service env vars `DB_HOST=127.0.0.1`,
`DB_PORT=3306`, `DB_USER=root`, `DB_PASSWORD=root`, `DB_NAME=scores365`,
`REDIS_URL=redis://127.0.0.1:6379/0`, `TEST_REDIS_URL=redis://127.0.0.1:6379/0`,
`JWT_SECRET=…`):

```bash
go test -tags=integration -count=1 -p 1 -timeout 300s ./...
```

The `//go:build integration` tag in files like
`internal/scraper/scores365/e2e_integration_test.go` is what keeps these
out of the default `go test ./...` run.

### Chain test (Docker)

The chain test installs every `.deb` in `dist/` in order and asserts
that the **last** one (the current proposal) comes up healthy. It catches
migration regressions and packaging breakages.

```bash
docker build -f deployments/docker/Dockerfile.test -t iptv-chain-test .
docker run --rm -p 8080:8080 -v "$PWD/dist:/tmp/dist:ro" iptv-chain-test
```

### Docker Compose for local dev

The example environment file lives at `deployments/docker/.env.example`
— copy it next to the compose file as `deployments/docker/.env` first
(compose resolves `.env` from the project directory, i.e. the folder
holding the compose file):

```bash
cp deployments/docker/.env.example deployments/docker/.env
docker compose -f deployments/docker/compose.dev.yml up --build
```

Three compose files live under `deployments/docker/`:

| File | Purpose |
|---|---|
| `compose.dev.yml` | Local development (one backend, persistent volumes). |
| `compose.test.yml` | CI / integration / E2E tests (tmpfs, bootstrap-invitation one-shot). |
| `compose.multi.yml` | Staging / load testing (three backends behind nginx + Prometheus). |

### End-to-end tests (Playwright)

The Playwright suite drives the running stack over HTTP and through the
dashboard. It needs the test stack up, the browser installed and the
dashboard built into the image (the compose file does that):

```bash
docker compose -f deployments/docker/compose.test.yml up --build --wait
cd web && npm ci && npx playwright install chromium
E2E_BASE_URL=http://127.0.0.1:8080 npx playwright test
```

`npx playwright test --project=api` runs only the API specs,
`--project=ui` only the browser ones.

The suite administers the stack with accounts of its own
(`e2e-admin@test.local`, `e2e-user@test.local`) and removes what it
creates, so a run leaves nothing behind and the next run starts from the
same state. The open upload sessions it tracks are aborted by the setup
of the next run, so an interrupted run does not exhaust the upload quota.

When the host port 8080 is taken, move the backend out of the way and
point the suite at it:

```bash
E2E_BACKEND_PORT=18081 docker compose -f deployments/docker/compose.test.yml up --build --wait
E2E_BASE_URL=http://127.0.0.1:18081 npx playwright test
```

Tear the stack down with
`docker compose -f deployments/docker/compose.test.yml down --volumes`.

### Auth round-trip smoke test

`scripts/auth_smoke/` is a tiny Go program that exercises every
`/api/web/v1/users/*` endpoint with the real protobuf wire format. It
needs a running backend (any of the three Docker setups above, or a
natively-run `go run ./cmd/server`) and a bootstrap invitation token you
paste into the source file:

```bash
go run ./scripts/auth_smoke/
```

The script prints each step's status (register 201, get-users 200,
invitation 201, refresh 200, logout 200) and is the recommended way to
verify the auth stack works end-to-end.

For the full production-like deploy (Ubuntu 24.04 + real `.deb` +
MariaDB + Redis + systemd-style env), see
[`../operations/production-like-local-deploy.md`](../operations/production-like-local-deploy.md).

### Auth wire format

Every `/api/web/v1/users/*` endpoint accepts and returns **protobuf
binary**, not JSON. The `Content-Type` for both request and response is
`application/x-protobuf`. The Vue dashboard and the Android client use
the generated bindings (`web/src/api/`, `flutter-apptv/lib/api/` in the
separate Flutter repo) and get this for free; if you `curl` these
endpoints, you need to encode the request with
`google.golang.org/protobuf/proto.Marshal` and decode the response with
`proto.Unmarshal`. Sending JSON yields zero-valued fields because
`proto.Unmarshal` silently ignores JSON bytes.

`/api/app/v1/crash-report` is the documented exception — it accepts
JSON because clients are Android crash reporters that do not use the
generated bindings (`internal/reporting/crash_handler.go` calls
`c.ShouldBindJSON`).

### Load test

See [`../performance/load-test.md`](../performance/load-test.md) for the
k6 setup, scenarios, and thresholds.

## Protobuf contract

The wire format is defined in
[`proto/api.proto`](../../proto/api.proto). Regenerate Go bindings after
editing:

```bash
make proto
```

`make proto-check` regenerates and fails if `internal/gen/api/` ends up
with uncommitted changes (the `ci.yml` workflow runs this same check
on `workflow_dispatch`).

The dashboard reads the same contract through the TypeScript bindings
in `web/src/proto/`. Regenerate them after editing the proto as well:

```bash
npm ci --prefix web   # the ts-proto plugin lives in the web workspace
make proto-web
```

`make proto-web-check` regenerates them and fails if they differ from
the committed ones. The committed bindings were produced with protoc
24.3 and ts-proto v2.11.5, the version `web/package.json` pins; the
version of protoc is recorded in the header of the generated file and
changes nothing else, so the check ignores that line.

The Flutter consumer (the Android TV app) lives in the separate
`flutter-apptv` repository and mirrors `proto/api.proto` verbatim.
Verify the mirror is in sync before tagging a release:

```bash
make proto-verify-flutter
```

If the Flutter proto is out of sync, copy `proto/api.proto` into the
Flutter repo (`../flutter-apptv/proto/api.proto`) and regenerate the
Dart bindings there (see
[`../integration-audit-roadmap.md`](../integration-audit-roadmap.md) for
the rollover plan that depends on this sync).

## Environment variables

See
[`../operations/README.md`](../operations/README.md#environment-variables)
for the single source of truth. Local development typically only needs
`DB_PASSWORD` and `JWT_SECRET` in `.env`; the rest have safe defaults.

## Conventions

- **Commit messages** — Conventional Commits in English, e.g.
  `feat(events): add cursor pagination to admin events query`.
- **Branch names** — `<type>/<short-kebab-description>`, e.g.
  `feat/scraper-403-cookie-refresh`.
- **PRs** — mergeable only when local `go test ./...`,
  `go test -tags=integration ./...`, and `cd web && npm ci && npm run lint && npm run build`
  pass. PR body must include what changed, why, and how it was tested.
- **Workflows** — the repository's four GitHub Actions workflows
  (`ci.yml`, `integration.yml`, `e2e.yml`, `load.yml`) are
  `workflow_dispatch`-only. They never gate a PR on its own; the local
  test suite is the source of truth.

## Where to escalate

- Architecture questions — open an issue tagged `architecture`.
- Operational concerns (anything that touches `deployments/` or the
  `.deb` package) — flag in the PR; the on-call operator will review.
- Security — see [`../../SECURITY.md`](../../SECURITY.md).
