# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| main    | :white_check_mark: Active development. Receives security fixes as soon as they are merged. |
| latest release tag | :white_check_mark: Receives security fixes for at least 30 days after the next tag is cut. |
| older   | :x: No longer maintained. Please upgrade. |

The application is deployed as a single rolling binary; there is no
long-term support branch. Operators who need an extended patch window
should pin to a specific release tag and follow the upgrade guide in
[`docs/operations/README.md`](docs/operations/README.md).

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security reports.**

Use one of the following private channels:

1. **GitHub Security Advisories** (preferred): click "Security" →
   "Advisories" → "New draft security advisory" on this repository.
2. **Email**: `security@jeriveromartinez.com`. Messages are routed to
   the maintainer and a backup reviewer.

Both channels are monitored. Email is the fallback if GitHub
Advisories is unavailable; include "SECURITY" in the subject line and
the affected commit SHA in the body so the report can be triaged
quickly.

### What to include

- A description of the vulnerability and its impact.
- Reproduction steps, ideally with a minimal request/response pair or a
  short script.
- The commit SHA, tag, or release version affected.
- Your contact handle and whether you would like to be credited in the
  advisory.

## Response Time

| Stage | Target |
|---|---|
| Initial acknowledgement | within 3 business days |
| Triage and impact assessment | within 7 business days |
| Patch for confirmed critical issues | within 14 days |
| Patch for confirmed high issues | within 30 days |
| Patch for confirmed medium/low issues | next regular release |

Critical issues are coordinated for a private fix first; the public
advisory and patch ship together to give operators a chance to upgrade.

## Scope

In scope:

- The `sofascore-scrapper` Go backend, the Vue web admin, and the
  scraper/scheduler under `internal/`.
- Configuration templates under `deployments/` (Docker, Nginx,
  Prometheus rules).
- Documentation that describes the security posture of the application
  (this file, the runbook, the development guide).

Out of scope:

- Third-party dependencies. Report upstream to the maintainer of the
  affected package; we will update the dependency in response.
- Social-engineering, denial-of-service, or physical-access scenarios.
- Issues that require the operator's own `JWT_SECRET` or database
  password to already be compromised.

## Operational Notes

Operators are expected to:

- Rotate `JWT_SECRET` and `DB_PASSWORD` on the schedule documented in
  [`docs/operations/runbook.md`](docs/operations/runbook.md).
- Run behind a reverse proxy that terminates TLS; the application does
  not serve TLS directly.
- Set `PPROF_ADDR` to a non-public interface or require the
  `PPROF_SECRET` documented in `internal/platform/observability/pprof.go`.
- Keep the `JWT_SECRET` value at least 32 bytes long; the application
  refuses to start with a shorter secret (see
  `internal/config/config.go`).
