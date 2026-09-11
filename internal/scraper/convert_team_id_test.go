package scraper

import (
	"testing"
	"time"
)

// TestToEvent_PopulatesHomeAndAwayTeamIdFKs is the regression test for
// fix B2 (PR #122). ToEvent was setting HomeTeamModel and AwayTeamModel
// (the relation *Team pointers) but NOT the HomeTeamId and AwayTeamId
// scalar FK columns on Event. Downstream queries that filter by team
// id (e.g. SELECT ... WHERE home_team_id = ?) returned zero rows for
// freshly scraped events even though the relation loaded fine.
func TestToEvent_PopulatesHomeAndAwayTeamIdFKs(t *testing.T) {
	start := time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)
	got := ToEvent(Match{
		Source:         "fotmob",
		SourceMatchId:  "4193492",
		Slug:           "team-a-vs-team-b",
		StartTimestamp: start,
		HomeTeam: Team{
			SourceId: 123,
			Name:     "Team A",
		},
		AwayTeam: Team{
			SourceId: 456,
			Name:     "Team B",
		},
		League: LeagueRef{
			Source:         "fotmob",
			SourceLeagueId: "47",
			Name:           "Premier League",
			Country:        "England",
			Sport:          "football",
		},
	}, "football")

	if got.HomeTeamId != 123 {
		t.Errorf("HomeTeamId: want 123, got %d", got.HomeTeamId)
	}
	if got.AwayTeamId != 456 {
		t.Errorf("AwayTeamId: want 456, got %d", got.AwayTeamId)
	}
	// Sanity: the relation pointers still work and reference the
	// same TeamId the FK columns carry.
	if got.HomeTeamModel == nil || got.HomeTeamModel.TeamId != 123 {
		t.Errorf("HomeTeamModel should point at TeamId 123, got %+v", got.HomeTeamModel)
	}
	if got.AwayTeamModel == nil || got.AwayTeamModel.TeamId != 456 {
		t.Errorf("AwayTeamModel should point at TeamId 456, got %+v", got.AwayTeamModel)
	}
}
