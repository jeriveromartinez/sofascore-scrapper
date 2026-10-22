# 365scores Scrapper — Product Overview

This document is the entry point for the **sales / commercial / pre-sales**
audience. It describes what the platform is, who it is for, what problems it
solves, and what it does **not** do. It avoids implementation details; for
those, see [`docs/development/README.md`](../development/README.md).

## What it is

A self-hosted backend platform that lets IPTV / OTT operators run a sports
content channel backed by [365scores](https://www.365scores.com/es-mx) and
[FotMob](https://www.fotmob.com/) data, served to operator-owned Android
set-top-box clients.

It bundles three things in one deployable package:

1. **Continuous event ingestion** from 365scores (`webws.365scores.com`)
   and FotMob (`www.fotmob.com`), normalized into a single event catalog.
2. **An operator admin dashboard** (Vue 3 SPA) for browsing events,
   managing devices, uploading APKs, and reviewing content stats.
3. **An Android APK distribution channel** — chunked, resumable uploads,
   versioned downloads with capability-token access and atomic activation.

The package is delivered as a single Ubuntu `.deb` (binary + dashboard +
systemd unit) so an operator can install and forget it.

## Who it is for

- **IPTV / OTT operators** who resell sports streaming to end users via
  Android TV boxes, sticks, or similar devices.
- **Small / mid-size operations** that want a self-managed backend without
  paying per-seat SaaS fees and without hosting customer data on a third
  party.

The platform is opinionated toward **single-region / single-operator**
deployments. Multi-tenant SaaS, billing, DRM, and content transcoding are
out of scope (see "What it does not do" below).

## Problems it solves

| Pain point | How the platform addresses it |
|---|---|
| Manual CSV / spreadsheet event schedules | Continuous ingestion from 365scores and FotMob; "today" events refresh every minute, next-7-days lookahead runs twice a day. |
| Hand-distributing APK updates to boxes | Versioned APK hosting with capability-token downloads; clients poll `GET /update` and pull new builds automatically. |
| No visibility into who is using the service | Per-device registration, per-playback event logging, content-stat aggregation, top-events reports. |
| Tying devices to specific content (tournaments / domains) | Per-device tournament assignments and a domain whitelist; a device only sees events for the tournaments it is assigned to. |
| Operational fragility when one server dies | Multi-instance deployment with shared MariaDB + Redis, distributed scheduler locks, atomic binary rollback. |

## Core capabilities

- **Live event ingestion** from two configured upstreams with a normalized
  catalog; per-league source routing via the `scraper_leagues` table, with
  an `override_source` escape hatch per row.
- **Admin SPA** (Vue 3 + Vite + Pinia + TypeScript) for event browsing,
  device roster, APK uploads, tournament assignment, domain whitelist, and
  content stats. JWT-authenticated, role-based (`admin` / `user`); the
  first registered user is promoted to `admin` automatically.
- **APK distribution** with chunked resumable upload, per-version metadata
  (semver, package name, active flag, download count), per-device download
  tokens with optional TTL, and atomic swap of the active version.
- **Device lifecycle**: self-registration via `POST /devices`; per-playback
  reporting via `POST /devices/viewing` feeding the content-stats
  aggregation; crash reports via `POST /crash-report` with rate limiting
  and body-size caps.
- **Analytics**: daily and monthly aggregation jobs (cron + Redis
  distributed lock); top-events ranking at `GET /stats/top-events`; a
  per-team logo cache served directly by the backend.
- **Ops primitives**: single binary + systemd unit (`iptv.service`);
  `/health/live` and `/health/ready`; `/metrics` (Prometheus); optional
  pprof debug server.

## Polling cadence

Two scrapers run concurrently:

- **Today (`scrapeTodaySpec = "@every 1m"`)** — fires every minute against
  the current UTC day. Soccer and most daily schedules renew fast enough
  that this is what fills the live board.
- **Next 7 days (`scrapeFutureSpec = "0 6,18 * * *"`)**: 06:00 and 18:00
  UTC, daily, for the lookahead fixture pull.

Both jobs are guarded by a Redis distributed lock so multiple backend
instances coordinate cleanly. The HTTP client caches each
`/web/games/allscores/` day-feed response for 60s so concurrent
per-league lookups inside the same tick coalesce.

The catalog of leagues is discovered **once at boot**; re-discovery is not
scheduled. Restart the backend to pick up leagues added upstream since the
last release.

## Hosting requirements

| Resource | Minimum | Notes |
|---|---|---|
| OS | Ubuntu 22.04 LTS or 24.04 LTS (amd64) | The `.deb` package targets these versions. |
| RAM | 2 GB | The scheduler, scraper, and HTTP server are all in one process. |
| Disk | 10 GB + APK storage | APK and image storage grow with content. |
| MariaDB | ≥ 10.5 | Provided as a `.deb` dependency. |
| Redis | ≥ 5.0 | Provided as a `.deb` dependency. |
| Outbound HTTPS | `webws.365scores.com`, `imagecache.365scores.com`, `www.fotmob.com`, `images.fotmob.com` | Required for the scraper. |
| Inbound | TCP 80 / TCP 8080 | TCP 80 if fronted by nginx; 8080 for direct dashboard access. |

A single host handles a single-tenant operator comfortably. Horizontal
scaling is supported (multi-instance ready, shared state in MariaDB and
Redis) but is not part of the standard install path.

## Observability

- `/health/live` — always `200` if the process is alive.
- `/health/ready` — checks MariaDB and Redis before returning `200`.
- `/metrics` — Prometheus.
- **pprof** is gated by **both** `ENABLE_PPROF=true` and `PPROF_ADDR=:port`.
  Setting only one does nothing; the server does not bind. Either leave both
  unset (default — disabled) or set both. See
  [`docs/operations/README.md`](../operations/README.md) for the env-var
  table.

## What it does **not** do

To set expectations honestly, the platform currently does **not** provide:

- **Content transcoding or DRM.** It distributes metadata and APKs; video
  stream origin and protection are the operator's responsibility.
- **Billing, subscriptions, or paywall logic.** No payment integration;
  the platform assumes an external billing system handles customer
  relationships.
- **End-user playback UI.** The Android TV client lives in a separate
  repository (`flutter-apptv`); the backend exposes the API the client
  consumes.
- **Multi-tenant SaaS mode.** One backend serves one operator; tenancy is
  not modeled in the schema.
- **Arbitrary data providers.** Sources are pluggable (365scores and FotMob
  ship in the box, routed per league from the admin catalog), but adding a
  new provider means writing a Go source adapter — it is not configuration.

If a customer needs any of those, treat the platform as a **foundation**,
not a turnkey product.

## Lifecycle (what the operator touches)

1. **Install** — `sudo apt install ./iptv_<version>_amd64.deb` on a fresh
   Ubuntu host. The postinst creates the `iptv` system user, generates
   `/etc/iptv/env` with random DB credentials and a JWT secret, creates
   the `iptv` database and user in MariaDB, and starts the service.
2. **Bootstrap** — log in to the dashboard using the bootstrap invitation
   token printed by `bootstrap-invitation`. The first registered user is
   promoted to admin automatically.
3. **Upload APK** — use the dashboard to upload the Android client APK
   (chunked upload, resumable). Activate the version when ready.
4. **Assign tournaments** — connect registered devices to the tournaments
   you want them to see.
5. **Operate** — watch `/health/ready` and `/metrics`; back up MariaDB
   daily; rotate `JWT_SECRET` and `DB_PASSWORD` if compromised.
6. **Upgrade** — install the new `.deb`; the postinst preserves
   `/etc/iptv/env` and stored data; the service restarts automatically.

For the full installation / upgrade / uninstall steps, see
[`docs/operations/deb-package.md`](../operations/deb-package.md).

## Talking points for pre-sales conversations

- **"It's open and self-hosted."** No per-seat fees; no third-party SaaS;
  customer data stays on the operator's hardware.
- **"It's a single binary on a single Ubuntu box."** No Kubernetes, no
  microservices, no service mesh — the operator needs only basic Linux
  sysadmin skills.
- **"It speaks protobuf to your Android app."** Wire-format contract is
  versioned and shared with the Flutter client; type-safety end-to-end.
- **"It already has the boring bits solved."** Migrations, distributed
  scheduler locks, atomic rollback, health checks, Prometheus metrics,
  rate limiting — all built in, not a roadmap item.
- **"You get an admin dashboard for free."** No separate admin UI project
  to scope, build, or maintain.

## Contact / escalation

For commercial questions, contact the project owner via GitHub (see
repository collaborators). For security disclosures, see the repository's
`SECURITY.md` (if present) or open a private advisory on GitHub.

For everything else, the entry points are:

- Sales / commercial: **this document**.
- Deployment & operations: [`docs/operations/README.md`](../operations/README.md).
- Development & contributing: [`docs/development/README.md`](../development/README.md).
