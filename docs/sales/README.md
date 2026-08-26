# SofaScore Scrapper — Product Overview

This document is the entry point for the **sales / commercial / pre-sales**
audience. It describes what the platform is, who it is for, what problems it
solves, and what it does **not** do. It avoids implementation details; for
those, see [`docs/development/README.md`](../development/README.md).

## What it is

A self-hosted backend platform that lets IPTV / OTT operators run a sports
content channel backed by [SofaScore](https://www.sofascore.com/) data and
their own Android-based set-top-box app.

It bundles three things in one deployable package:

1. A backend service that continuously ingests sports events from SofaScore.
2. An admin web dashboard for the operator (event browsing, device and APK
   management, content analytics).
3. An Android APK distribution channel (resumable uploads, versioned
   downloads, capability-token access).

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
| Manual CSV / spreadsheet event schedules | Continuous scraping of SofaScore; events refresh every minute for today and every 12 hours for the next 7 days. |
| Hand-distributing APK updates to boxes | Versioned APK hosting with capability-token downloads; clients poll `GET /update` and pull new builds automatically. |
| No visibility into who is using the service | Per-device registration, per-playback event logging, content-stat aggregation, top-events reports. |
| Tying devices to specific content (tournaments / domains) | Per-device tournament assignments and a domain whitelist; a device only sees events for tournaments it is assigned to. |
| Operational fragility when one server dies | Multi-instance deployment with shared MariaDB + Redis, distributed scheduler locks, atomic binary rollback. |

## Core capabilities

### Live sports event ingestion
- Scrapes SofaScore continuously (`chromedp`-based) with tunable concurrency.
- Caches "current events" for fast device polling; cache invalidates by an
  epoch counter, not by TTL.
- Captures match status (`scheduled` / `live` / `finished`) so the client UI
  can show "LIVE" / "FINAL" badges.

### Admin web dashboard
- Built-in SPA (Vue 3 + Vite + Pinia + TypeScript).
- Browser-based event browser, device roster, APK upload, tournament
  assignment, domain whitelist, content stats.
- JWT-authenticated, role-based (`admin` / `user`); first registered user is
  promoted to admin automatically.

### APK distribution
- Chunked, resumable upload from the dashboard.
- Per-version metadata (semver, package name, active flag, download count).
- Per-device download tokens with optional TTL.
- Atomic swap of active version; devices detect updates via `GET /update`.

### Device lifecycle
- Self-registration via `POST /devices`; each device gets a server-assigned
  ID and a tournament assignment.
- Per-playback reporting (`POST /devices/viewing`) feeds the content-stats
  aggregation.
- Crash reports (`POST /crash-report`) with IP rate limiting and a body-size
  cap.

### Analytics
- Daily and monthly aggregation jobs (cron + Redis distributed lock).
- Top-events ranking exposed via `GET /stats/top-events`.
- Per-team logo cache served directly by the backend.

### Operations
- Single binary, single systemd unit (`iptv.service`).
- Health endpoints: `/health/live` (always 200 if process alive) and
  `/health/ready` (checks DB + Redis).
- Prometheus metrics at `/metrics`; optional pprof endpoint.
- Native production deploy via `deployments/native/deploy.sh` with atomic
  rollback to the previous binary.

## Hosting requirements

| Resource | Minimum | Notes |
|---|---|---|
| OS | Ubuntu 22.04 LTS or 24.04 LTS (amd64) | The `.deb` package targets these versions. |
| RAM | 2 GB | The scheduler, scraper and HTTP server are all in one process. |
| Disk | 10 GB + APK storage | `apk_storage` and `image_storage` grow with content. |
| MariaDB | 10.5+ | Provided automatically as a `.deb` dependency. |
| Redis | 5.0+ | Provided automatically as a `.deb` dependency. |
| Outbound HTTPS | `www.sofascore.com` | Required for the scraper. |
| Inbound | TCP 80 / TCP 8080 | TCP 80 if fronted by nginx; 8080 for direct dashboard access. |

A single host handles a single-tenant operator comfortably. Horizontal
scaling is supported (the codebase is multi-instance ready) but is not part
of the standard install path.

## What it does **not** do

To set expectations honestly, the platform currently does **not** provide:

- **Content transcoding or DRM.** It distributes metadata and APKs; video
  stream origin and protection are the operator's responsibility.
- **Billing, subscriptions, or paywall logic.** No payment integration; the
  platform assumes an external billing system handles customer
  relationships.
- **End-user playback UI.** The Android TV client lives in a separate
  repository (`flutter-apptv`); the backend exposes the API the client
  consumes.
- **Multi-tenant SaaS mode.** One backend serves one operator; tenancy is
  not modeled in the schema.
- **SofaScore alternatives.** The scraper is hard-coded to SofaScore; there
  is no pluggable source framework.

If a customer needs any of those, treat the platform as a **foundation**,
not a turnkey product.

## Lifecycle (what the operator touches)

1. **Install** — `sudo apt install ./iptv_<version>_amd64.deb` on a fresh
   Ubuntu host. The postinst creates the `iptv` system user, generates
   `/etc/iptv/env` with random DB credentials and a JWT secret, creates the
   `iptv` database and user in MariaDB, and starts the service.
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
