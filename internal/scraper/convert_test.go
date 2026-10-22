package scraper

import (
	"testing"
	"time"
)

func timestampMatch(start time.Time) Match {
	return Match{
		Source:         "fotmob",
		SourceMatchId:  "4193492",
		Slug:           "chelsea-vs-arsenal",
		StartTimestamp: start,
		HomeTeam:       Team{SourceId: 1, Name: "H"},
		AwayTeam:       Team{SourceId: 2, Name: "A"},
		League:         LeagueRef{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "England", Sport: "football"},
	}
}

func TestToEvent_ConvertsTimestampToMilliseconds(t *testing.T) {
	event, err := ToEvent(timestampMatch(time.Unix(1710000000, 0)), "football")
	if err != nil {
		t.Fatalf("ToEvent: %v", err)
	}
	if event.StartTimestamp != 1710000000000 {
		t.Errorf("StartTimestamp: want 1710000000000, got %d", event.StartTimestamp)
	}
}

func TestToEvent_GuardsUnusableTimestamps(t *testing.T) {
	cases := []struct {
		name  string
		start time.Time
	}{
		{"zero time", time.Time{}},
		{"before the epoch", time.Unix(-1, 0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := ToEvent(timestampMatch(tc.start), "football")
			if err != nil {
				t.Fatalf("ToEvent: %v", err)
			}
			if event.StartTimestamp != 0 {
				t.Errorf("StartTimestamp: want 0, got %d", event.StartTimestamp)
			}
		})
	}
}

func TestToEvent_GuardsOverflowingTimestamp(t *testing.T) {
	event, err := ToEvent(timestampMatch(time.Unix(5e15, 0)), "football")
	if err != nil {
		t.Fatalf("ToEvent: %v", err)
	}
	if event.StartTimestamp != 0 {
		t.Errorf("StartTimestamp: want 0 (overflow guarded), got %d", event.StartTimestamp)
	}
}

func TestToEvent_FromMatch(t *testing.T) {
	m := Match{
		Source:         "fotmob",
		SourceMatchId:  "4193492",
		Slug:           "chelsea-vs-arsenal",
		StartTimestamp: time.Unix(1735689600, 0),
		Status:         MatchStatus{Code: 1, Type: "scheduled"},
		HomeTeam:       Team{SourceId: 8455, Name: "Chelsea", LogoURL: "https://x/chelsea.png"},
		AwayTeam:       Team{SourceId: 9825, Name: "Arsenal", LogoURL: "https://x/arsenal.png"},
		League:         LeagueRef{SourceLeagueId: "47", Name: "Premier League"},
	}
	e, err := ToEvent(m, "football")
	if err != nil {
		t.Fatalf("ToEvent: %v", err)
	}
	if e.ExternalMatchId != "4193492" {
		t.Fatalf("ExternalMatchId: %q", e.ExternalMatchId)
	}
	if e.Source != "fotmob" {
		t.Fatalf("Source: %q", e.Source)
	}
	if e.HomeTeamModel == nil || e.HomeTeamModel.Name != "Chelsea" {
		t.Fatalf("HomeTeamModel: %+v", e.HomeTeamModel)
	}
}
