// Package sportsdb wires the TheSportsDB public API as a scraper
// Source for non-football sports (NBA, NFL, MLB, NHL, etc.). The
// underlying client lives in internal/sportsdb and is shared with
// the logo-lookup feature (PR #130). The two surfaces read from
// the same in-memory cache + rate limiter, so a multi-sport cron
// does not burst through the free-tier 30 req/min quota.
package sportsdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/sportsdb"
)

// Source implements scraper.Source on top of TheSportsDB's
// eventsday.php endpoint. The endpoint supports both a
// league-filtered form (`l=<id>`) and a day-wide form (no
// filter), so the source implements both DayMatcher (for the
// bulk path) and Source.ScheduledEvents (per-league).
//
// PR #133 extended the source to bulk-fetch every event for a
// day in a single HTTP call. The free-tier quota on
// eventsday.php is 3 req/min, so this collapses the per-league
// fan-out (NBA/NFL/MLB/NHL = 4 req/min) into one req/min and
// leaves room for future sports without re-tuning the cron.
type Source struct {
	client *sportsdb.Client
	logger *slog.Logger
}

// TeamIDPrefix namespaces TheSportsDB team IDs into the shared
// `teams` table so they cannot collide with FotMob team IDs
// (which range from 0 to ~1.7M in our observed dataset). The
// prefix 2_000_000_000 sits well above any FotMob ID and below
// math.MaxInt64, so adding `sportsdb_id + TeamIDPrefix` is safe
// even for 6-7 digit upstream IDs. The DB unique index is on
// `team_id` alone (no `source` column), so prefixing is the
// schema-free way to keep two sources' rows distinct.
const TeamIDPrefix int64 = 2_000_000_000

// NewSource wraps a sportsdb.Client with the Source interface.
// The logger is optional and falls back to slog.Default().
func NewSource(client *sportsdb.Client) *Source {
	return &Source{client: client, logger: slog.Default()}
}

func (s *Source) Name() string { return "sportsdb" }

// DayMatches fetches every event TheSportsDB publishes for the
// given date — across every sport, every league, every match —
// in a single HTTP round-trip. The implementation uses the
// unfiltered form of eventsday.php; the dispatch layer is
// responsible for partitioning the result by league and routing
// each bucket to the catalog.
//
// The returned matches carry LeagueRef.SourceLeagueId as the
// only field the upstream guarantees. Name/Sport/Country are
// populated from the upstream event payload (canonical sport
// via sportsdb.NormalizeSport) as a fallback so the dispatch
// layer can persist even before the catalog has been
// populated by the discovery job.
func (s *Source) DayMatches(ctx context.Context, date time.Time) ([]scraper.Match, error) {
	dateStr := date.Format("2006-01-02")
	raw, err := s.client.EventsDayAll(ctx, dateStr)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: day-all on %s: %w", dateStr, err)
	}
	out := make([]scraper.Match, 0, len(raw))
	for _, ev := range raw {
		if ev.Postponed {
			continue
		}
		if ev.IDLeague == "" {
			// Event has no league binding (shouldn't happen
			// but be defensive). Skip rather than dump it
			// under a synthetic league.
			continue
		}
		out = append(out, s.eventToMatch(ev))
	}
	return out, nil
}

// ScheduledEvents fetches the day's events for a single league
// and converts them to scraper.Match. Postponed matches
// (strPostponed:"yes") are dropped — the daily upsert would
// otherwise overwrite a rescheduled fixture with stale data.
func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	day, err := s.fetchLeagueDay(ctx, league, date)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
	return day, nil
}

// fetchLeagueDay pulls one league's day payload and converts
// each Event into a scraper.Match. Used by ScheduledEvents; the
// day-wide DayMatches path calls EventsDayAll instead.
func (s *Source) fetchLeagueDay(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	dateStr := date.Format("2006-01-02")
	events, err := s.client.EventsByDay(ctx, league.SourceLeagueId, dateStr)
	if err != nil {
		return nil, err
	}
	out := make([]scraper.Match, 0, len(events))
	for _, raw := range events {
		if raw.Postponed {
			continue
		}
		out = append(out, s.toMatch(league, raw))
	}
	return out, nil
}

// toMatch is the per-league path: it uses the catalog-supplied
// LeagueRef so the persisted sport stays canonical (lowercase
// "basketball" not TheSportsDB's verbose "Basketball").
func (s *Source) toMatch(league scraper.LeagueRef, raw sportsdb.Event) scraper.Match {
	matchLeague := scraper.LeagueRef{
		Source:         s.Name(),
		SourceLeagueId: league.SourceLeagueId,
		Name:           raw.League,
		Sport:          league.Sport,
		Country:        league.Country,
	}
	return scraper.Match{
		Source:         s.Name(),
		SourceMatchId:  raw.IDEvent,
		StartTimestamp: raw.Timestamp,
		HomeScore:      raw.HomeScore,
		AwayScore:      raw.AwayScore,
		Status: scraper.MatchStatus{
			Finished:  !raw.Timestamp.After(time.Now()) && (raw.HomeScore != 0 || raw.AwayScore != 0),
			Started:   !raw.Timestamp.After(time.Now()),
			Cancelled: false,
		},
		HomeTeam: s.teamFromEvent(raw.HomeTeam, raw.IDHomeTeam, raw.HomeTeamBadge),
		AwayTeam: s.teamFromEvent(raw.AwayTeam, raw.IDAwayTeam, raw.AwayTeamBadge),
		League:   matchLeague,
	}
}

// eventToMatch is the day-wide path: it does not know which
// league came from the operator's catalog, so it uses the
// upstream's idLeague / strLeague / strSport / strCountry and
// canonicalises the sport via sportsdb.NormalizeSport. The
// dispatch loop overrides the sport with the catalog's value
// if the row exists there.
func (s *Source) eventToMatch(raw sportsdb.Event) scraper.Match {
	matchLeague := scraper.LeagueRef{
		Source:         s.Name(),
		SourceLeagueId: raw.IDLeague,
		Name:           raw.League,
		Sport:          sportsdb.NormalizeSport(raw.Sport),
		Country:        raw.Country,
	}
	return scraper.Match{
		Source:         s.Name(),
		SourceMatchId:  raw.IDEvent,
		StartTimestamp: raw.Timestamp,
		HomeScore:      raw.HomeScore,
		AwayScore:      raw.AwayScore,
		Status: scraper.MatchStatus{
			Finished:  !raw.Timestamp.After(time.Now()) && (raw.HomeScore != 0 || raw.AwayScore != 0),
			Started:   !raw.Timestamp.After(time.Now()),
			Cancelled: false,
		},
		HomeTeam: s.teamFromEvent(raw.HomeTeam, raw.IDHomeTeam, raw.HomeTeamBadge),
		AwayTeam: s.teamFromEvent(raw.AwayTeam, raw.IDAwayTeam, raw.AwayTeamBadge),
		League:   matchLeague,
	}
}

// teamFromEvent builds a scraper.Team from the upstream event's
// name, ID, and badge URL. The ID is prefixed with TeamIDPrefix so
// the row lands in the shared `teams` table without colliding with
// any FotMob-sourced row (see TeamIDPrefix comment).
//
// When the upstream omits the ID (older fixtures) or badge (teams
// still being indexed), we leave SourceId=0 / LogoURL="" so the
// event still persists — the team row will simply not link to a
// logo download and the UI will fall back to the placeholder.
func (s *Source) teamFromEvent(name, idRaw, badge string) scraper.Team {
	var sourceID int64
	if idRaw != "" {
		if n, err := strconv.ParseInt(idRaw, 10, 64); err == nil && n > 0 {
			sourceID = n + TeamIDPrefix
		}
	}
	return scraper.Team{
		SourceId: sourceID,
		Name:     name,
		LogoURL:  badge,
	}
}

// SearchLeagues is intentionally not implemented: sportsdb
// leagues are curated via the admin catalog (seeded entry per
// sport) so operators always know exactly which leagues they
// are scraping.
func (s *Source) SearchLeagues(_ context.Context, _ string) ([]scraper.LeagueSearchResult, error) {
	return nil, errors.New("sportsdb: SearchLeagues is not supported; add leagues via the catalog admin form")
}
