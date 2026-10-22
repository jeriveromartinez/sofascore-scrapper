package scraper

import (
	"testing"
	"time"
)

// TestTournamentID_NamespacesUpstreamLeagueIDs is the regression
// test for the shared primary key: the tournaments table also holds
// the rows the admin UI creates, and those get auto-increment ids
// starting at 1. A FotMob league id used verbatim as the primary key
// therefore collides with the admin tournament that carries the same
// number.
func TestTournamentID_NamespacesUpstreamLeagueIDs(t *testing.T) {
	fotmob, err := TournamentID(LeagueRef{Source: "fotmob", SourceLeagueId: "47"})
	if err != nil {
		t.Fatalf("fotmob league: %v", err)
	}
	if fotmob != uint(FotMobTournamentIDBase)+47 {
		t.Errorf("fotmob tournament id = %d, want %d", fotmob, uint(FotMobTournamentIDBase)+47)
	}
	if fotmob == 47 {
		t.Error("fotmob tournament id must not reuse the raw league id")
	}

	// The 365scores ids already carry the upstream prefix and stay
	// where they are.
	scores365, err := TournamentID(LeagueRef{Source: "scores365", SourceLeagueId: "6000000047"})
	if err != nil {
		t.Fatalf("scores365 league: %v", err)
	}
	if scores365 != 6000000047 {
		t.Errorf("scores365 tournament id = %d, want 6000000047", scores365)
	}

	if fotmob == scores365 {
		t.Errorf("the two sources must not share a tournament id: %d", fotmob)
	}
}

// TestToTournament_MalformedLeagueIDFails is the regression test for
// the silent primary key 0: a league id that is not a number used to
// become 0, and the tournaments table then collected every league
// whose id the upstream failed to publish under the same row.
func TestToTournament_MalformedLeagueIDFails(t *testing.T) {
	for _, id := range []string{"", "not-a-number", "0", "-1", "12.5"} {
		if _, err := ToTournament(LeagueRef{Source: "fotmob", SourceLeagueId: id}); err == nil {
			t.Errorf("league id %q: expected an error, got none", id)
		}
	}
}

// TestToScrapeBatch_MalformedLeagueIDFails makes sure the failure
// reaches the caller instead of producing a batch with a zero-id
// tournament.
func TestToScrapeBatch_MalformedLeagueIDFails(t *testing.T) {
	batch, err := ToScrapeBatch([]Match{
		{
			Source:         "fotmob",
			SourceMatchId:  "1",
			StartTimestamp: time.Unix(1735689600, 0),
			HomeTeam:       Team{SourceId: 1, Name: "H"},
			AwayTeam:       Team{SourceId: 2, Name: "A"},
			League:         LeagueRef{Source: "fotmob", SourceLeagueId: "not-a-number", Name: "L"},
		},
	}, "football")
	if err == nil {
		t.Fatal("expected the malformed league id to fail the batch")
	}
	if len(batch.Events) != 0 || len(batch.Tournaments) != 0 {
		t.Errorf("a failed batch must be empty, got %d events and %d tournaments",
			len(batch.Events), len(batch.Tournaments))
	}
}
