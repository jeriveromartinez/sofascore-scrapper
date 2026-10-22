# scores365-scrapper

IPTV backend service that scrapes sports events from 365scores and
FotMob, manages APK distribution, device registration, playback
tracking, and serves the web admin dashboard.

## Documentation by audience

| If you are… | Start here |
|---|---|
| **Sales / commercial / pre-sales** — what the platform is, who it is for, what it solves, what it does not do | [`docs/sales/README.md`](docs/sales/README.md) |
| **Operations / sysadmin / SRE** — install, upgrade, monitor, troubleshoot, roll back | [`docs/operations/README.md`](docs/operations/README.md) |
| **Developer / engineer** — build, test, contribute, work on the proto contract | [`docs/development/README.md`](docs/development/README.md) |

Operations also keeps [`runbook.md`](docs/operations/runbook.md) (alert
responses), [`troubleshooting.md`](docs/operations/troubleshooting.md)
(symptom-to-fix), and the other guides linked from
[`docs/operations/README.md`](docs/operations/README.md).

## Stack at a glance

- **Backend** — Go 1.25, Gin, GORM (MariaDB), go-redis, robfig/cron. Schema
  is owned by GORM AutoMigrate; there are no migration files.
- **Frontend** — Vue 3, Vite, Pinia, TypeScript. Playwright for E2E, Vitest
  for unit.
- **Infrastructure** — Docker multi-stage builds, nginx, Prometheus.
- **Wire format** — Protocol Buffers v3. Single source of truth at
  [`proto/api.proto`](proto/api.proto); Go bindings regenerate into
  `internal/gen/api/`.

On first boot the server creates the schema with GORM AutoMigrate and
seeds a default admin (`admin@local`). Its password comes from the
`DEFAULT_ADMIN_PASSWORD` environment variable; when that variable is
unset the seeder generates a random password and never prints it, so
either set `DEFAULT_ADMIN_PASSWORD` before the first boot or register
the first account with the `bootstrap-invitation` command.

The project layout, environment variables, local setup, build and test
commands, protobuf contract, and scheduled jobs are detailed in
[`docs/development/README.md`](docs/development/README.md).
