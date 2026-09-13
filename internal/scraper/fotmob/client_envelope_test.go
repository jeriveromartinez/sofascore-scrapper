package fotmob

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestApiMatchesResponse_UnmarshalsLeaguesEnvelope is the regression
// test for PR #124. FotMob's real `/api/data/matches` endpoint returns
// the day's matches grouped under `leagues[].matches[]` (NOT the
// `{matches:{allMatches:[]}}` envelope that fix B1 (PR #122) assumed
// for the legacy `/api/leagues` endpoint).
//
// The fixture under testdata/matches_envelope.json models the real
// wire payload (two leagues, three matches total) and the test
// asserts:
//   - the response unmarshals without an error;
//   - the leagues are split per FotMob grouping;
//   - the first Bundesliga match has its home team name round-tripped
//     ("Union Berlin") and the int64 ID decoded correctly;
//   - the score halves (home.score / away.score) round-trip as ints
//     even when the upstream string (`status.scoreStr`) is "1 - 3"
//     with whitespace — the canonical source of truth is the int
//     fields on Home/Away;
//   - the status.reason.short string ("FT") round-trips on the
//     nested Status (used to populate scraper.MatchStatus.Type).
func TestApiMatchesResponse_UnmarshalsLeaguesEnvelope(t *testing.T) {
	path := filepath.Join("testdata", "matches_envelope.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	var resp apiMatchesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if got := len(resp.Leagues); got != 2 {
		t.Fatalf("Leagues len: want 2, got %d", got)
	}

	bundesliga := resp.Leagues[0]
	if bundesliga.Id != 54 {
		t.Errorf("bundesliga Id: want 54, got %d", bundesliga.Id)
	}
	if bundesliga.Name != "Bundesliga" {
		t.Errorf("bundesliga Name: want Bundesliga, got %q", bundesliga.Name)
	}
	if got := len(bundesliga.Matches); got != 2 {
		t.Fatalf("bundesliga Matches len: want 2, got %d", got)
	}

	first := bundesliga.Matches[0]
	if first.Id != 5881169 {
		t.Errorf("first match Id: want 5881169, got %d", first.Id)
	}
	if first.Home.Name != "Union Berlin" {
		t.Errorf("first match home name: want Union Berlin, got %q", first.Home.Name)
	}
	if first.Home.Score != 1 {
		t.Errorf("first match home score: want 1, got %d", first.Home.Score)
	}
	if first.Away.Score != 3 {
		t.Errorf("first match away score: want 3, got %d", first.Away.Score)
	}
	if first.StatusId != 6 {
		t.Errorf("first match StatusId: want 6, got %d", first.StatusId)
	}
	if first.Status.ScoreStr != "1 - 3" {
		t.Errorf("first match status.scoreStr: want %q, got %q", "1 - 3", first.Status.ScoreStr)
	}
	if first.Status.Finished != true {
		t.Errorf("first match status.finished: want true, got %v", first.Status.Finished)
	}
	if first.Status.Reason == nil {
		t.Fatalf("first match status.reason should not be nil")
	}
	if got, _ := first.Status.Reason["short"].(string); got != "FT" {
		t.Errorf("first match status.reason.short: want FT, got %q", got)
	}

	laliga := resp.Leagues[1]
	if laliga.Id != 87 {
		t.Errorf("laliga Id: want 87, got %d", laliga.Id)
	}
	if got := len(laliga.Matches); got != 1 {
		t.Fatalf("laliga Matches len: want 1, got %d", got)
	}
}

// TestApiMatchesResponse_RejectsLegacyEnvelope guards against a
// future refactor that tries to model the response as the legacy
// `{matches:{allMatches:[]}}` envelope from fix B1 (PR #122).
// The real FotMob /api/data/matches endpoint returns `leagues[]` as
// the top-level key, so a legacy-envelope payload must produce an
// empty Leagues slice — the calling code filters by league ID and
// would otherwise see zero matches silently.
func TestApiMatchesResponse_RejectsLegacyEnvelope(t *testing.T) {
	legacy := []byte(`{"matches":{"allMatches":[{"id":"4193492","home":{"id":1,"name":"H"},"away":{"id":2,"name":"A"}}]}}`)
	var resp apiMatchesResponse
	if err := json.Unmarshal(legacy, &resp); err != nil {
		t.Logf("unmarshal legacy envelope errored (ok): %v", err)
	}
	if got := len(resp.Leagues); got != 0 {
		t.Fatalf("Leagues should be 0 for legacy envelope input, got %d", got)
	}
}

// TestApiMatchesResponse_RejectsBareArray guards against a future
// refactor that tries to model the response as `[]apiMatch` directly.
// The real FotMob response is wrapped in `{leagues[], date}`, so a
// bare-array decode must not silently produce zero rows that "look"
// successful in the calling code.
func TestApiMatchesResponse_RejectsBareArray(t *testing.T) {
	bare := []byte(`[{"id":1}]`)
	var resp apiMatchesResponse
	if err := json.Unmarshal(bare, &resp); err != nil {
		// A struct with a slice field should not error on a bare
		// array; instead it should leave the slice empty. Either
		// outcome is acceptable for this guard test, what matters
		// is that the Leagues count is zero.
		t.Logf("unmarshal bare array errored (ok): %v", err)
	}
	if got := len(resp.Leagues); got != 0 {
		t.Fatalf("Leagues should be 0 for bare-array input, got %d", got)
	}
}
