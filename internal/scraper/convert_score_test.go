package scraper

import (
	"testing"
	"time"
)

// TestToEvent_PassesHomeAndAwayScore covers PR #124. The previous
// implementation parsed ScoreStr on MatchStatus into HomeScore /
// AwayScore. The FotMob /api/data/matches endpoint exposes per-team
// ints directly, so the scraper carries them on Match.HomeScore and
// Match.AwayScore and ToEvent must read them from there. ToEvent
// MUST NOT silently default to 0-0 for finished matches that carry
// real scores.
func TestToEvent_PassesHomeAndAwayScore(t *testing.T) {
	start := time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)
	home := Team{SourceId: 1, Name: "H"}
	away := Team{SourceId: 2, Name: "A"}
	league := LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "England", Sport: "football"}

	cases := []struct {
		name      string
		homeScore int
		awayScore int
	}{
		{"home win", 3, 1},
		{"away win", 0, 2},
		{"draw", 1, 1},
		{"zero-zero scheduled", 0, 0},
		{"single goal", 1, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToEvent(Match{
				Source:         "fotmob",
				SourceMatchId:  "4193492",
				Slug:           "x",
				StartTimestamp: start,
				HomeScore:      tc.homeScore,
				AwayScore:      tc.awayScore,
				HomeTeam:       home,
				AwayTeam:       away,
				League:         league,
				Status: MatchStatus{
					Started:  true,
					Finished: true,
					ScoreStr: "", // empty: int halves are authoritative
				},
			}, "football")
			if got.HomeScore != tc.homeScore {
				t.Errorf("HomeScore: want %d, got %d", tc.homeScore, got.HomeScore)
			}
			if got.AwayScore != tc.awayScore {
				t.Errorf("AwayScore: want %d, got %d", tc.awayScore, got.AwayScore)
			}
		})
	}
}

// TestToEvent_DefaultsZeroScoresOnScheduledMatch verifies that a
// scheduled match (not started yet) leaves HomeScore / AwayScore at
// 0 even if ScoreStr happens to carry a "0-0" string — int halves
// on Match are authoritative.
func TestToEvent_DefaultsZeroScoresOnScheduledMatch(t *testing.T) {
	start := time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)
	home := Team{SourceId: 1, Name: "H"}
	away := Team{SourceId: 2, Name: "A"}
	league := LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "England", Sport: "football"}

	got := ToEvent(Match{
		Source:         "fotmob",
		SourceMatchId:  "4193492",
		Slug:           "x",
		StartTimestamp: start,
		HomeScore:      0,
		AwayScore:      0,
		HomeTeam:       home,
		AwayTeam:       away,
		League:         league,
		Status: MatchStatus{
			Started:  false,
			Finished: false,
			ScoreStr: "0-0",
		},
	}, "football")
	if got.HomeScore != 0 {
		t.Errorf("HomeScore: want 0, got %d", got.HomeScore)
	}
	if got.AwayScore != 0 {
		t.Errorf("AwayScore: want 0, got %d", got.AwayScore)
	}
	if got.StatusType != "notstarted" {
		t.Errorf("StatusType: want notstarted, got %q", got.StatusType)
	}
}
