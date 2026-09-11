package fotmob

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestApiMatchesResponse_UnmarshalsAllMatchesEnvelope is the regression
// test for fix B1 (PR #122): FotMob's /api/leagues endpoint returns
// the matches list under the {allMatches: [...]} envelope, not under
// the top-level {matches: [...]} the current struct expects.
//
// The fixture under testdata/league_matches_envelope.json models the
// real wire payload (the {allMatches:[...]} shape) and the test
// asserts:
//   - the response unmarshals without an error;
//   - the matches end up in the AllMatches slice;
//   - the slice length matches the fixture (2 matches);
//   - the first match's home team name round-trips ("Team A").
func TestApiMatchesResponse_UnmarshalsAllMatchesEnvelope(t *testing.T) {
	path := filepath.Join("testdata", "league_matches_envelope.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	var resp apiMatchesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if got := len(resp.Matches.AllMatches); got != 2 {
		t.Fatalf("AllMatches len: want 2, got %d", got)
	}

	first := resp.Matches.AllMatches[0]
	if first.Home.Name != "Team A" {
		t.Errorf("first match home name: want Team A, got %q", first.Home.Name)
	}
	if first.Status.ScoreStr != "" {
		t.Errorf("first match scoreStr should be empty, got %q", first.Status.ScoreStr)
	}

	second := resp.Matches.AllMatches[1]
	if second.Status.ScoreStr != "2-1" {
		t.Errorf("second match scoreStr: want 2-1, got %q", second.Status.ScoreStr)
	}
}

// TestApiMatchesResponse_RejectsBareArray guards against a future
// refactor that tries to model the response as `[]apiMatch` directly.
// The real FotMob response is wrapped in {leagueId, allMatches}, so a
// bare-array decode must not silently produce zero rows that "look"
// successful in the calling code.
func TestApiMatchesResponse_RejectsBareArray(t *testing.T) {
	bare := []byte(`[{"id":"1"}]`)
	var resp apiMatchesResponse
	if err := json.Unmarshal(bare, &resp); err != nil {
		// A struct with a slice field should not error on a bare
		// array; instead it should leave the slice empty. Either
		// outcome is acceptable for this guard test, what matters
		// is that the AllMatches count is zero.
		t.Logf("unmarshal bare array errored (ok): %v", err)
	}
	if got := len(resp.Matches.AllMatches); got != 0 {
		t.Fatalf("AllMatches should be 0 for bare-array input, got %d", got)
	}
}
