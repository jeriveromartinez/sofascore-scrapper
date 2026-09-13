package scraper

import (
	"context"
	"time"
)

type Source interface {
	Name() string
	ScheduledEvents(ctx context.Context, league LeagueRef, date time.Time) ([]Match, error)
	SearchLeagues(ctx context.Context, query string) ([]LeagueSearchResult, error)
}

// DayMatcher is the optional interface that sources implement when
// they can return every match for a date in a single HTTP round-trip.
//
// The FotMob /api/data/matches endpoint serves the full per-day
// payload in one call (grouped per league). Without this interface
// the scheduler would issue one HTTP request per (league, date) pair
// — 41 leagues × 1 cron tick per minute × 1440 minutes ≈ 59k
// requests/day, with the client rate-limit dropping actual
// throughput to ~5 req/s. The scheduler uses DayMatcher when
// available and falls back to ScheduledEvents when not.
type DayMatcher interface {
	DayMatches(ctx context.Context, date time.Time) ([]Match, error)
}

type LeagueRef struct {
	Source         string
	SourceLeagueId string
	Name           string
	Country        string
	Sport          string
}

type LeagueSearchResult struct {
	Source         string
	SourceLeagueId string
	Name           string
	Country        string
	Sport          string
}

type MatchStatus struct {
	Code        int
	Description string
	// Type is the source's short status label (e.g. "FT" for
	// full-time on FotMob /api/data/matches). Empty when the
	// upstream payload does not provide one.
	Type      string
	Finished  bool
	Started   bool
	Cancelled bool
	// ScoreStr is the source-supplied score string (e.g. "2-1").
	// Kept for backwards compatibility with sources that only
	// expose a free-form score string; the canonical score halves
	// live on Match.HomeScore and Match.AwayScore and ToEvent
	// reads those directly. Empty when the source has no score
	// yet (scheduled matches, etc.) or when the source exposes
	// scores only as ints.
	ScoreStr string
}

type Team struct {
	SourceId       int64
	Name           string
	LogoURL        string
	PrimaryColor   string
	SecondaryColor string
	TextColor      string
}

type Match struct {
	Source         string
	SourceMatchId  string
	Slug           string
	StartTimestamp time.Time
	// HomeScore and AwayScore are the source's authoritative
	// scores. PR #124: the FotMob /api/data/matches payload
	// exposes per-team `score` ints directly, so we carry them
	// through as ints instead of parsing them out of a
	// status-side ScoreStr string. They default to 0 when the
	// match has not started yet.
	HomeScore int
	AwayScore int
	Status    MatchStatus
	HomeTeam  Team
	AwayTeam  Team
	League    LeagueRef
}
