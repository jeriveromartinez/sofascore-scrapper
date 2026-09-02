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

- **Backend:** Go 1.25, Gin, GORM (MariaDB), go-redis, golang-migrate, robfig/cron
- **Frontend:** Vue 3, Vite, Pinia, TypeScript, Playwright (E2E), Vitest (unit)
- **Infrastructure:** Docker (multi-stage), nginx, Prometheus
- **Wire format:** Protocol Buffers v3; single source of truth in [`proto/api.proto`](proto/api.proto)
