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
	Type        string
	Finished    bool
	Started     bool
	Cancelled   bool
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
	Status         MatchStatus
	HomeTeam       Team
	AwayTeam       Team
	League         LeagueRef
}
