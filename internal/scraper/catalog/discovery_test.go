// internal/scraper/catalog/discovery_test.go
package catalog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/sportsdb"
	"gorm.io/gorm"
)

// newTestRepo builds an in-memory catalog repo. Reuses the
// package's own AutoMigrate via the existing newTestDB helper
// where possible, but discovery.go needs its own
// setup because some tests want a fresh schema.
func newDiscoveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&ScraperLeague{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestDiscovery_InsertsAllLeaguesFromUpstream covers the happy
// path: the upstream returns 3 leagues and every one of them
// ends up in the catalog on a fresh DB.
func TestDiscovery_InsertsAllLeaguesFromUpstream(t *testing.T) {
	repo := NewRepository(newDiscoveryDB(t))
	server := newAllLeaguesServer(`
		{"countrys":null,"leagues":[
			{"idLeague":"4387","strLeague":"NBA","strSport":"Basketball","strCountry":"USA"},
			{"idLeague":"4391","strLeague":"NFL","strSport":"American Football","strCountry":"USA"},
			{"idLeague":"4380","strLeague":"NHL","strSport":"Ice Hockey","strCountry":"USA"}
		],"sports":null}`)
	defer server.Close()
	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})

	res, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if res.Upserted != 3 {
		t.Errorf("Upserted = %d, want 3", res.Upserted)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", res.Skipped)
	}
	if res.TotalSeen != 3 {
		t.Errorf("TotalSeen = %d, want 3", res.TotalSeen)
	}

	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("catalog rows = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if !r.Enabled {
			t.Errorf("row %s: enabled=false on a fresh upsert (should default to true so operator can choose to disable)", r.SourceLeagueId)
		}
	}
}

// TestDiscovery_IsIdempotent runs Discovery twice and asserts
// the second run inserts zero new rows. This is the contract
// the weekly cron relies on — if it ever stops being true
// the catalog table will grow without bound.
func TestDiscovery_IsIdempotent(t *testing.T) {
	repo := NewRepository(newDiscoveryDB(t))
	server := newAllLeaguesServer(`
		{"leagues":[
			{"idLeague":"4387","strLeague":"NBA","strSport":"Basketball","strCountry":"USA"}
		]}`)
	defer server.Close()
	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})

	first, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.Upserted != 1 {
		t.Fatalf("first Upserted = %d, want 1", first.Upserted)
	}
	second, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Upserted != 0 {
		t.Errorf("second Upserted = %d, want 0 (idempotency broken)", second.Upserted)
	}
	if second.Unchanged != 1 {
		t.Errorf("second Unchanged = %d, want 1", second.Unchanged)
	}
}

// TestDiscovery_NormalizesSport ensures upstream verbose
// strings ("American Football") are stored in canonical form
// ("american-football") so the admin sport filter works.
func TestDiscovery_NormalizesSport(t *testing.T) {
	repo := NewRepository(newDiscoveryDB(t))
	server := newAllLeaguesServer(`
		{"leagues":[
			{"idLeague":"4391","strLeague":"NFL","strSport":"American Football","strCountry":"USA"}
		]}`)
	defer server.Close()
	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})

	if _, err := Discovery(context.Background(), repo, client, nil); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Sport != "american-football" {
		t.Errorf("Sport = %q, want american-football", rows[0].Sport)
	}
}

// TestDiscovery_SkipsEmptyIDs defends the contract that the
// upstream sometimes publishes an entry with idLeague=""
// (the parent league or banner row). Those must never make
// it into the catalog with a blank primary key. The client's
// AllLeagues decoder drops them upstream of Discovery, so the
// test asserts the catalog still ends up with exactly the
// non-empty IDs and Discovery reports TotalSeen matching the
// filtered slice.
func TestDiscovery_SkipsEmptyIDs(t *testing.T) {
	repo := NewRepository(newDiscoveryDB(t))
	server := newAllLeaguesServer(`
		{"leagues":[
			{"idLeague":"","strLeague":"Unknown","strSport":"Mystery","strCountry":"X"},
			{"idLeague":"4387","strLeague":"NBA","strSport":"Basketball","strCountry":"USA"}
		]}`)
	defer server.Close()
	client := sportsdb.NewClient(sportsdb.Options{BaseURL: server.URL + "/api/v1/json/3"})

	res, err := Discovery(context.Background(), repo, client, nil)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0 (the decoder filters empty IDs upstream of Discovery)", res.Skipped)
	}
	if res.Upserted != 1 {
		t.Errorf("Upserted = %d, want 1", res.Upserted)
	}
	if res.TotalSeen != 1 {
		t.Errorf("TotalSeen = %d, want 1 (empty IDs dropped by the decoder)", res.TotalSeen)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.SourceLeagueId == "" {
			t.Fatalf("catalog contains a row with empty source_league_id: %+v", r)
		}
	}
}

// newAllLeaguesServer wires a fixture server that responds to
// search_all_leagues.php?s= (the path the sportsdb client
// AllLeagues path takes today). Returns a handler that
// streams the body verbatim and counts hits.
func newAllLeaguesServer(body string) *httptest.Server {
	var hits int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		// Return the same shape the real upstream returns so
		// the client's decoder is exercised end-to-end.
		if err := json.Unmarshal([]byte(body), &map[string]any{}); err != nil {
			http.Error(w, "fixture invalid: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
}
