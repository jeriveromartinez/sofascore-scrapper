# sofascore-scrapper

IPTV backend service — scrapes sports events from SofaScore, manages
APK distribution, device registration, playback tracking, and
provides a web dashboard.

## Documentation by audience

| If you are… | Start here |
|---|---|
| **Sales / commercial / pre-sales** — explaining what the platform is, who it is for, what problems it solves, and what it explicitly does not do | [`docs/sales/README.md`](docs/sales/README.md) |
| **Operations / sysadmin / SRE** — installing, upgrading, monitoring, troubleshooting, rolling back | [`docs/operations/README.md`](docs/operations/README.md) |
| **Developer / engineer** — building, testing, contributing code, working on the proto contract | [`docs/development/README.md`](docs/development/README.md) |

Each entry point links to the detailed guides (`.deb` packaging,
runbook, rollback procedures, load tests, architecture, API
reference, protobuf contract, etc.).

## Stack at a glance

- **Backend:** Go 1.25, Gin, GORM (MariaDB), go-redis, robfig/cron (no migrations — GORM AutoMigrate owns schema)
- **Frontend:** Vue 3, Vite, Pinia, TypeScript, Playwright (E2E), Vitest (unit)
- **Infrastructure:** Docker (multi-stage), nginx, Prometheus
- **Wire format:** Protocol Buffers v3; single source of truth in [`proto/api.proto`](proto/api.proto)

On first boot the server auto-creates the schema via GORM AutoMigrate and seeds a default admin account (`admin@local` / `admin1234`). Change the password on first login.

The full project layout, environment variables, local setup, build / test /
lint commands, API reference, protobuf contract, data model, and scheduled
jobs live in [`docs/development/README.md`](docs/development/README.md).

## Operations

Day-to-day operations (alerts, Redis outage behavior, counter recovery,
cache invalidation) are documented in
[`docs/operations/runbook.md`](docs/operations/runbook.md). Rollback
procedures (migration policy, backup/restore, destructive migration
warnings) are in [`docs/operations/rollback.md`](docs/operations/rollback.md).
The Ubuntu `.deb` packaging guide (build, install, upgrade, uninstall) is
in [`docs/operations/deb-package.md`](docs/operations/deb-package.md). Start
with [`docs/operations/README.md`](docs/operations/README.md) for the
deployer entry point.
