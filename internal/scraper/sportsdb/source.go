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
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/sportsdb"
)

// Source implements scraper.Source on top of TheSportsDB's
// eventsday.php endpoint. The endpoint is per-league
// (`l=<leagueID>`), so the source does NOT implement
// scraper.DayMatcher — the Service routes each configured
// league through ScheduledEvents instead.
//
// The shared rate limiter (in sportsdb.Client) spaces these
// per-league calls 5s apart, well below the 30 req/min free-tier
// quota. With 4 non-football leagues (NBA/NFL/MLB/NHL) the
// backfill is one HTTP call every 5s = ~20s total.
type Source struct {
	client *sportsdb.Client
	logger *slog.Logger
}

// NewSource wraps a sportsdb.Client with the Source interface.
// The logger is optional and falls back to slog.Default().
func NewSource(client *sportsdb.Client) *Source {
	return &Source{client: client, logger: slog.Default()}
}

func (s *Source) Name() string { return "sportsdb" }

// ScheduledEvents fetches the day's events for a single league
// and converts them to scraper.Match. Postponed matches
// (strPostponed="yes") are dropped — the daily upsert would
// otherwise overwrite a rescheduled fixture with stale data.
func (s *Source) ScheduledEvents(ctx context.Context, league scraper.LeagueRef, date time.Time) ([]scraper.Match, error) {
	day, err := s.fetchLeagueDay(ctx, league, date)
	if err != nil {
		return nil, fmt.Errorf("sportsdb: %s on %s: %w", league.SourceLeagueId, date.Format("2006-01-02"), err)
	}
	return day, nil
}

// fetchLeagueDay pulls one league's day payload and converts
// each Event into a scraper.Match.
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

func (s *Source) toMatch(league scraper.LeagueRef, raw sportsdb.Event) scraper.Match {
	// Use the league passed in by the Service so we honour the
	// canonical sport string ("basketball", "american-football",
	// etc.) the operator configured in the catalog. TheSportsDB's
	// own strSport is uppercase + verbose ("Basketball"), and
	// mixing the two would break sport filters in the API layer.
	matchLeague := scraper.LeagueRef{
		Source:         s.Name(),
		SourceLeagueId: league.SourceLeagueId,
		Name:           raw.League,
		Sport:          league.Sport,
		Country:        league.Country,
	}
	return scraper.Match{
		Source:        s.Name(),
		SourceMatchId: raw.IDEvent,
		StartTimestamp: raw.Timestamp,
		HomeScore:     raw.HomeScore,
		AwayScore:     raw.AwayScore,
		Status: scraper.MatchStatus{
			Finished:  !raw.Timestamp.After(time.Now()) && (raw.HomeScore != 0 || raw.AwayScore != 0),
			Started:   !raw.Timestamp.After(time.Now()),
			Cancelled: false,
		},
		HomeTeam: scraper.Team{
			SourceId: 0,
			Name:     raw.HomeTeam,
		},
		AwayTeam: scraper.Team{
			SourceId: 0,
			Name:     raw.AwayTeam,
		},
		League: matchLeague,
	}
}

// SearchLeagues is intentionally not implemented: sportsdb
// leagues are curated via the admin catalog (seeded entry per
// sport) so operators always know exactly which leagues they
// are scraping.
func (s *Source) SearchLeagues(_ context.Context, _ string) ([]scraper.LeagueSearchResult, error) {
	return nil, errors.New("sportsdb: SearchLeagues is not supported; add leagues via the catalog admin form")
}
