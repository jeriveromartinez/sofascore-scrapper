package scraper

import (
	"testing"
	"time"
)

// TestToEvent_NormalizesFotMobStatus covers the four mapping branches
// defined for fix B3 (PR #122). The repository only selects events
// with status_type in {notstarted, inprogress}, so ToEvent must rewrite
// the FotMob status bool-triple (started, finished, cancelled) into
// those repository values. The mapping:
//
//   - started && finished      -> "finished"
//   - started && !finished     -> "inprogress"
//   - !started && !finished    -> "notstarted"
//   - cancelled                -> "cancelled"
//   - anything else            -> ""
//
// Anything outside the four known states is normalized to the empty
// string so it does not silently match repository WHERE clauses.
func TestToEvent_NormalizesFotMobStatus(t *testing.T) {
	start := time.Date(2026, 9, 10, 20, 0, 0, 0, time.UTC)
	home := Team{SourceId: 1, Name: "H"}
	away := Team{SourceId: 2, Name: "A"}
	league := LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "England", Sport: "football"}

	cases := []struct {
		name   string
		status MatchStatus
		want   string
	}{
		{"scheduled becomes notstarted", MatchStatus{Started: false, Finished: false, Cancelled: false}, "notstarted"},
		{"live becomes inprogress", MatchStatus{Started: true, Finished: false, Cancelled: false}, "inprogress"},
		{"played becomes finished", MatchStatus{Started: true, Finished: true, Cancelled: false}, "finished"},
		{"cancelled takes precedence", MatchStatus{Started: true, Finished: false, Cancelled: true}, "cancelled"},
		{"cancelled without started still wins", MatchStatus{Started: false, Finished: false, Cancelled: true}, "cancelled"},
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
				Status:         tc.status,
			}, "football")
			if got.StatusType != tc.want {
				t.Errorf("StatusType: want %q, got %q", tc.want, got.StatusType)
			}
		})
	}
}
