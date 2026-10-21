# Verifying scraper output in production

Queries to confirm that events are actually reaching the database, and
which source produced them. Run against the production MySQL/MariaDB
instance.

Schema assumptions (see `internal/events/model.go`,
`internal/tournaments/model.go`, `internal/scraper/convert.go`):

- There is **no** `scraper_leagues` table. Leagues land in
  `tournaments`, keyed by `id = parseLeagueID(source.SourceLeagueId)`.
- `tournaments` has no `source` or `sport` column — only `name`, `slug`,
  `region`.
- `events.source` (`varchar(32)`) and `events.sport` are the only places
  the origin of an event is recorded.

## 1. Are events being written at all?

An empty result, or a `MAX(scraped_at)` far in the past, means the
scraper stopped — start with the scraper logs before looking anywhere
else.

```sql
SELECT
    source,
    sport,
    COUNT(*)                    AS events,
    MAX(scraped_at)             AS last_scrape_ms,
    FROM_UNIXTIME(MAX(scraped_at) / 1000) AS last_scrape_utc
FROM events
GROUP BY source, sport
ORDER BY events DESC;
```

Read `last_scrape_utc`: it should be within one cron interval of now. A
row with thousands of events and a stale timestamp means ingestion
stopped after a successful historical run.

## 2. Is one source starving the other?

The multi-source dispatcher fans out to every configured source and
merges the results. If one source starts returning nothing, the merged
output shrinks silently — the rows from the healthy source look normal
in isolation, so always compare the two side by side.

```sql
SELECT
    source,
    COUNT(DISTINCT league_id)                       AS leagues,
    COUNT(*)                                        AS events,
    FROM_UNIXTIME(MAX(start_timestamp) / 1000)      AS latest_kickoff_utc
FROM events
GROUP BY source;
```

If a source that was previously present has dropped out of this result
entirely, check the scraper logs for the dispatcher warnings added in
PR #140 (`scrape day: unknown league from source`, `scraper: no active
leagues for source`) — those are the signals for exactly this failure.

## 3. Are leagues being populated?

A source with leagues but zero events is scraping league metadata and
failing to match any fixtures inside them, which is a different problem
from a source that has no leagues at all.

```sql
SELECT
    t.id,
    t.name,
    t.slug,
    t.region,
    COUNT(e.id) AS events
FROM tournaments t
LEFT JOIN events e ON e.league_id = t.id
GROUP BY t.id, t.name, t.slug, t.region
ORDER BY events ASC
LIMIT 50;
```

Leagues with `events = 0` at the bottom of this list are candidates for
removal from the source catalog, or for a sport-mapping bug — a league
configured under the wrong `sport` will never match.

## 4. Data quality spot-checks

```sql
-- Events with no score despite being finished: extraction gap.
SELECT external_match_id, source, sport, status_type, home_score, away_score
FROM events
WHERE status_type = 'finished' AND (home_score = 0 AND away_score = 0)
ORDER BY scraped_at DESC
LIMIT 20;

-- Events with a zero/absent kickoff time: timestamp extraction failed.
SELECT COUNT(*) AS events_without_kickoff
FROM events
WHERE start_timestamp = 0;
```

A non-zero `events_without_kickoff` points at the timestamp handling in
`internal/scraper/convert.go` (`ToEvent` zeroes out-of-range values
rather than failing), not at the source payload.

## 5. Team logo coverage

```sql
SELECT
    COUNT(*) AS teams,
    SUM(CASE WHEN logo_url LIKE '/teams/logo/%' THEN 1 ELSE 0 END) AS proxied,
    SUM(CASE WHEN logo_url LIKE 'http%' THEN 1 ELSE 0 END)         AS still_remote
FROM teams;
```

`still_remote` should trend toward zero once the logo scheduler has
caught up. A large persistent value means downloads are failing — check
the `events: failed to download logo for team` log lines.