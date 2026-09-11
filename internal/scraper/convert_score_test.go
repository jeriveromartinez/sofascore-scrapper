package scraper

import (
	"testing"
	"time"
)

// TestToEvent_ParsesScoreStr covers fix B4 (PR #122). ToEvent was
// dropping scores entirely: the FotMob apiStatus.ScoreStr (e.g.
// "2-1") was never carried through to events.Event.HomeScore and
// AwayScore, so the API always reported 0-0 even for finished
// matches. The fix parses ScoreStr on MatchStatus into the two
// integer score fields. Empty or malformed values default to 0.
func TestToEvent_ParsesScoreStr(t *testing.T) {
	start := time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)
	home := Team{SourceId: 1, Name: "H"}
	away := Team{SourceId: 2, Name: "A"}
	league := LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "England", Sport: "football"}

	cases := []struct {
		name       string
		scoreStr   string
		wantHome   int
		wantAway   int
	}{
		{"home win", "3-1", 3, 1},
		{"away win", "0-2", 0, 2},
		{"draw", "1-1", 1, 1},
		{"zero-zero scheduled", "0-0", 0, 0},
		{"empty stays zero", "", 0, 0},
		{"malformed stays zero", "live", 0, 0},
		{"too few sides stays zero", "1", 0, 0},
		{"non-numeric stays zero", "a-b", 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToEvent(Match{
				Source:         "fotmob",
				SourceMatchId:  "4193492",
				Slug:           "x",
				StartTimestamp: start,
				HomeTeam:       home,
				AwayTeam:       away,
				League:         league,
				Status: MatchStatus{
					Started:  true,
					Finished: true,
					ScoreStr: tc.scoreStr,
				},
			}, "football")
			if got.HomeScore != tc.wantHome {
				t.Errorf("HomeScore: want %d, got %d", tc.wantHome, got.HomeScore)
			}
			if got.AwayScore != tc.wantAway {
				t.Errorf("AwayScore: want %d, got %d", tc.wantAway, got.AwayScore)
			}
		})
	}
}
