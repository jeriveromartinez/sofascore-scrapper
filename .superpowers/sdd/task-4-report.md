# Task 4 Report: Corregir ranking y agregaciones

## Summary

Fixed 3 bugs in the reporting package: ranking query, daily aggregation SQL compatibility, and monthly aggregation period selection.

## Changes

### `repository.go` — GetTopEvents

| Before | After |
|--------|-------|
| `SELECT content, count(*)` — column not mapped to `SofaScoreEventId` | `SELECT CAST(content AS UNSIGNED) AS sofa_score_event_id` |
| No numeric filter — non-numeric content included | `WHERE content REGEXP '^[0-9]+$'` |
| `ORDER BY view_count DESC` — ties undeterministic | `ORDER BY view_count DESC, sofa_score_event_id ASC` |
| No limit cap | Capped at 100 (0 or negative defaults to 100) |
| No DB error propagation | Propagates `result.Error` |

SQLite compatibility: uses `CAST(content AS INTEGER)` + `content NOT GLOB '*[^0-9]*'` fallback for tests.

### `aggregation_repository.go` — GenerateDaily

| Before | After |
|--------|-------|
| `CAST(ended_at AS SIGNED)` not supported in SQLite | Removed unnecessary casts |
| `DIV 1000` MySQL-only | `/ 1000` (compatible with both MySQL and SQLite) |

### `aggregation_repository.go` — GenerateMonthly

| Before | After |
|--------|-------|
| `WHERE created_at >= ? AND created_at <= ?` | `WHERE period_start >= ? AND period_start < ?` (half-open range) |
| `end := begin.AddDate(0,1,0).Add(-time.Second)` | `end := begin.AddDate(0,1,0)` (clean half-open) |
| `ctx.Save(&monthStats)` — creates duplicates | `ctx.Clauses(clause.OnConflict{...}).Create(...)` — upsert on `(content_hash, period_type, period_start)` |
| Not idempotent | Deletes old monthly rows before insert |
| Daily delete uses `created_at` | Daily delete uses `period_start >= ? AND period_start < ?` |

## Tests (13 total, all passing)

`repository_integration_test.go`:
- `TestGetTopEvents_NumericContentOnly` — non-numeric content excluded
- `TestGetTopEvents_DeterministicOrdering` — ORDER BY view_count DESC, sofa_score_event_id ASC
- `TestGetTopEvents_CapAt100` — limit > 100 capped
- `TestGetTopEvents_ZeroLimitDefaultsTo100` — limit=0 defaults to 100
- `TestGetTopEvents_ReturnsDBError` — propagates errors

`aggregation_integration_test.go`:
- `TestGenerateDaily_CreatesContentStats` — daily aggregation creates ContentStat rows
- `TestGenerateDaily_DeletesProcessedLogs` — processed logs deleted
- `TestGenerateDaily_MillisecondDuration` — ms to seconds conversion correct
- `TestGenerateMonthly_UsesPeriodStart` — correct period_start filtering
- `TestGenerateMonthly_HalfOpenRange` — `>= begin AND < end`
- `TestGenerateMonthly_Idempotent` — re-runnable without side effects
- `TestGenerateMonthly_DeletesDailyRows` — daily rows cleaned up
- `TestGenerateMonthly_EmptyPeriodDoesNotFail` — empty input doesn't error

## Verification

```
$ go test -tags=integration ./internal/reporting/... -count=1
ok  github.com/jeriveromartinez/sofascore-scrapper/internal/reporting  0.776s
```
