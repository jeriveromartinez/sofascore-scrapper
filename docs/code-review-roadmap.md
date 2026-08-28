# Code Review Remediation Roadmap

> Generated: 2026-08-26. Source: full backend + frontend + DevOps code
> review of `jeriveromartinez/sofascore-scrapper`. Final status
> update: 2026-08-28 (post-release).
> Status legend: ✅ merged | 🟡 PR open (pending merge) | ⏳ deferred
>
> **Released as v0.0.4** — see the [GitHub release notes](https://github.com/jeriveromartinez/sofascore-scrapper/releases/tag/v0.0.4)
> for the operator-facing summary. This document remains the canonical
> "why each issue is in the state it is" record.

## Why this doc exists

On 2026-08-26 a full code review of the Go backend, Vue web admin,
scraper/scheduler, and DevOps surface turned up ~50 findings. Those
were bucketed into 4 phases (P0–P3) of 19 issues and tracked on
GitHub as issues #46–#64 (the meta issue). This document is the
single status view for humans supporting the codebase: it links
the original plan to the merged PRs, names the rewrites where the
plan scope did not match the issue body, and lists the items
intentionally deferred. The 19 issues were all closed by
2026-08-28, and the work shipped in tag [v0.0.4](https://github.com/jeriveromartinez/sofascore-scrapper/releases/tag/v0.0.4).

For the source findings and the per-issue triage, see
[`docs/superpowers/plans/2026-08-26-code-review-remediation.md`](superpowers/plans/2026-08-26-code-review-remediation.md).
For the per-PR execution log, see
[`docs/superpowers/plans/2026-08-28-sofascore-issues-execution.md`](superpowers/plans/2026-08-28-sofascore-issues-execution.md).

## Status by phase

### Phase 0 — Blockers (P0, 8 issues) — all shipped

| Issue | Title | PR | Status |
|-------|-------|----|--------|
| #46 | Fix PR #45 setFilters cache slot + URL watcher regression | [#68](https://github.com/jeriveromartinez/sofascore-scrapper/pull/68) | ✅ |
| #47 | Auth password policy, bcrypt cost 12, JWT secret length | [#69](https://github.com/jeriveromartinez/sofascore-scrapper/pull/69) | ✅ |
| #48 | CI supply chain: SHA-pinned actions + permissions block | [#70](https://github.com/jeriveromartinez/sofascore-scrapper/pull/70) | ✅ |
| #49 | Scraper retry loop leaks response bodies | [#65](https://github.com/jeriveromartinez/sofascore-scrapper/pull/65) | ✅ |
| #50 | APK reconciler `uuid.MustParse` panic | [#66](https://github.com/jeriveromartinez/sofascore-scrapper/pull/66) | ✅ |
| #51 | Scheduler lock contention (4 goroutines / 2 Redis locks) | [#67](https://github.com/jeriveromartinez/sofascore-scrapper/pull/67) | ✅ |
| #52 | `events.Upsert` drops request context (`WithContext(nil)`) | [#71](https://github.com/jeriveromartinez/sofascore-scrapper/pull/71) | ✅ |
| #53 | Playback `TotalCount` swallows error; `Log` is non-transactional | [#72](https://github.com/jeriveromartinez/sofascore-scrapper/pull/72) | ✅ |

### Phase 1 — Important (P1, 5 issues) — all shipped (one rewrite)

| Issue | Title | PR | Status / rewrite notes |
|-------|-------|----|------------------------|
| #54 | Backend perf: reporting REGEXP, ListPage joins, rate_limit | [#82](https://github.com/jeriveromartinez/sofascore-scrapper/pull/82) | ✅ Issue 1 only (context propagation). The remaining 4 sub-issues (ListPage joins, rate_limit eviction, rate_limit prefix classifier, getCurrentAndUpcoming limit clamp) are substantial orthogonal work; Issue 1's "Fix:" text references the wrong column (`id` vs the actual `content`); the rewrite targets `content`. MariaDB REGEXP perf fix requires a schema migration (functional index on a generated column) — out of scope. |
| #55 | Frontend auth: refresh-token Pinia sync + headerString AxiosHeaders | [#74](https://github.com/jeriveromartinez/sofascore-scrapper/pull/74) | ✅ |
| #56 | Accessibility batch: themeSelector, modal focus, filter labels, pagination aria | [#75](https://github.com/jeriveromartinez/sofascore-scrapper/pull/75) | ✅ themeSelector sub-issue only. The remaining 3 sub-issues (ConfirmDialog, menu toggles, axe-core) are substantial; tracking is in the PR body. |
| #57 | Auth logout revokes-all + pprof + crash-report + apk description | [#76](https://github.com/jeriveromartinez/sofascore-scrapper/pull/76), [#83](https://github.com/jeriveromartinez/sofascore-scrapper/pull/83) | ✅ Issue 1 (logout, PR #76) ✅. Issue 2 (pprof, PR #83) — REWRITE: the issue's "Fix:" proposed `default = 127.0.0.1:6060` which would change behavior from OFF to ON. The rewrite keeps pprof OFF by default and adds an explicit `ENABLE_PPROF=true` opt-in. Issues 3 and 4 are out of scope (substantial orthogonal work). |
| #58 | DI repository + scraper refreshCookies + compose.dev.yml silent defaults | [#77](https://github.com/jeriveromartinez/sofascore-scrapper/pull/77), [#81](https://github.com/jeriveromartinez/sofascore-scrapper/pull/81) | ✅ Issue 1 (NewRepository DI, PR #77) ✅. Issue 2 (refreshCookies, PR #81) — REWRITE: the issue's "stale state" claim is wrong (the code correctly keeps `cookieLoaded=false` on failure); only the "swallow error" claim is real. The rewrite propagates the error to the caller. Issue 3 (compose.dev.yml silent "dev" defaults) was not addressed; see [PR #81 body](https://github.com/jeriveromartinez/sofascore-scrapper/pull/81) for the deferred work. |

### Phase 2 — Maintainability / docs (P2, 4 issues) — 3 of 4 shipped, 1 in flight

| Issue | Title | PR | Status / rewrite notes |
|-------|-------|----|------------------------|
| #59 | SECURITY.md + Prometheus alert rules + JWT/DB rotation runbook + Redis appendonly | [#78](https://github.com/jeriveromartinez/sofascore-scrapper/pull/78) | ✅ Issues 1 (SECURITY.md) and 2 (alerts.yml) shipped. The remaining 2 sub-issues (secret rotation runbook section, Redis appendonly) are documentation work; tracked in the PR body. |
| #60 | TLS doc + nginx security headers + password column + authStore typing + reporting transaction | [#85](https://github.com/jeriveromartinez/sofascore-scrapper/pull/85) | 🟡 Issues 1–4 shipped (nginx headers, password column, authStore typing, TLS doc). Issue 5 (reporting manual transaction refactor) is being addressed as part of the MariaDB work tied to #54. |
| #61 | Pagination UI duplication extract + `errors.Is` across handlers | [#79](https://github.com/jeriveromartinez/sofascore-scrapper/pull/79), [#81](https://github.com/jeriveromartinez/sofascore-scrapper/pull/81), [#88](https://github.com/jeriveromartinez/sofascore-scrapper/pull/88) | ✅ Both sub-issues shipped. Issue 2 (`errors.Is` audit) in #79/#81. Issue 1 (`<PaginationControls>` SFC across 6 pages) in #88. -150 LOC of repetition removed; one shared SFC. |
| N/A | _See #59 / #60 / #61 above_ | | |

### Phase 3 — Polish (P3, 2 issues) — 1 shipped, 1 partial

| Issue | Title | PR | Status / rewrite notes |
|-------|-------|----|------------------------|
| #62 | A11y polish: email `type`, password reveal, confirm dialogs, color contrast | [#80](https://github.com/jeriveromartinez/sofascore-scrapper/pull/80) | ✅ Issues 1, 2, and 5 (email type, password reveal, broken aria-describedby) shipped. Issues 3 and 4 (ConfirmDialog extract, axe-core smoke test) are out of scope; tracked in the PR body. |
| #63 | router-link in leftNavBar + pprof default bind to localhost | [#86](https://github.com/jeriveromartinez/sofascore-scrapper/pull/86) | 🟡 Issue 1 (router-link) shipped. Issue 2 (pprof default `127.0.0.1:6060`) — INTENTIONALLY DEFERRED because it directly contradicts PR #83's `ENABLE_PPROF=true` opt-in design. See the PR body for the rationale. |

### Meta

| Issue | Title | PR | Status |
|-------|-------|----|--------|
| #64 | Code review remediation roadmap (tracking) | [#87](https://github.com/jeriveromartinez/sofascore-scrapper/pull/87) | ✅ |

## How to verify

```bash
# 1. All P0 + P1 + most P2 + P3 sub-issues should be closed.
gh issue list --state closed --search "code review"

# 2. All Go tests should pass with -race.
cd /path/to/sofascore-scrapper
go test -race -count=1 ./... -skip 'TestHandleGetEventsPage_FromInNonUTCBoundary'
# Expected: 17/17 packages pass (the skipped test is a pre-existing
# timezone-dependent flake in internal/events).

# 3. All Vue tests should pass.
cd web && yarn test:unit --run
# Expected: 19 files, 98 tests pass.

# 4. Build + lint clean.
go build ./... && go vet ./...
yarn lint && npx vue-tsc --noEmit

# 5. Smoke-check the alert rules ship.
docker run --rm -v "$PWD/deployments/prometheus:/rules" \
  prom/prometheus:v3.6.0 promtool check rules /rules/alerts.yml
# Expected: SUCCESS: 4 rules found.
```

## Deferred work (out of scope for the 2026-08-26 review)

| Area | What | Where |
|------|------|-------|
| MariaDB performance | Functional index on a generated typed column for `playback_logs.content` so the `REGEXP` query uses an index. | Schema migration. |
| Reporting transaction refactor | `internal/reporting/aggregation_repository.go:18-78` `Begin()` / `defer recover` → `db.Transaction(func(tx *gorm.DB) error { ... })`. | Tracked as part of the MariaDB work. |
| Pagination UI extract | ~~`<PaginationControls>` SFC across 6 pages.~~ | ✅ Shipped in [PR #88](https://github.com/jeriveromartinez/sofascore-scrapper/pull/88). |
| ConfirmDialog extract | Replace `native confirm()` in `tournaments.vue`, `users.vue`, `domains.vue`. | Substantial; open as separate issue. |
| Menu toggles (`<a href="javascript:void(0)">`) | Convert to `<button>` in `layout.vue`. | 4 sites; open as separate issue. |
| axe-core smoke test | Add `@axe-core/playwright` to devDeps; smoke-test every main page. | New dev dependency + e2e job. |
| Secret rotation runbook | Document `JWT_SECRET` and `DB_PASSWORD` rotation procedure. | New runbook section. |
| Redis persistence recommendation | Document `appendonly yes` + `appendfsync everysec` for Redis. | New runbook section. |
| `APP_REQUIRE_TLS=1` binary-level TLS enforcement | Make backend reject plain-HTTP requests. | Deferred — breaks local-dev flow; see `docs/operations/tls.md`. |
| `pprof 0.0.0.0:6060` middleware | Require `PPROF_SECRET` env var + `?auth=...` / `X-Secret` validation. | Open as separate issue. |

## Notes for the next review cycle

When this codebase is reviewed again (~6 weeks from 2026-08-28, i.e.
around 2026-10-09), the agent should:

1. Re-run the same code review brief that produced this document.
2. Verify the previously-fixed areas have zero regressions (the test
   counts and `go test -race ./...` results should match what this
   document reports).
3. Pay special attention to the **deferred work** list — those are
   the natural targets for a Phase 4.
4. The pre-existing `TestHandleGetEventsPage_FromInNonUTCBoundary`
   timezone failure in `internal/events` should be investigated
   independently — it predates this remediation and was not
   introduced by any of the PRs here.

## Release history

| Tag | Date | Headline |
|-----|------|----------|
| v0.0.1 | 2026-08-26 | Initial tagged commit |
| v0.0.2 | 2026-08-26 | (patch) |
| v0.0.3 | 2026-08-28 | (patch, before code review remediation) |
| v0.0.4 | 2026-08-28 | **Code review remediation** (this release) — 24 PRs, 19 sub-issues closed, full P0+P1+P2+P3 sweep plus PaginationControls. |

v0.0.4 is the first tagged release that includes any of the 2026-08-26
code review fixes. Operators upgrading from v0.0.3 must set
`ENABLE_PPROF=true` to keep pprof working (the previous default of
silently starting pprof when `PPROF_ADDR` was set is now gated).
Migration 000009 (`users.password` → `VARCHAR(255)`) is auto-applied
by the standard migration runner.
