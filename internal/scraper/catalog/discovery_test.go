// internal/scraper/catalog/discovery_test.go
package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"gorm.io/gorm"
)

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

// competitionsBody is a trimmed /web/competitions/ response: two
// tennis competitions, one basketball competition, and one football
// competition that must be filtered out (FotMob owns football).
const competitionsBody = `{
  "countries": [{"id":1,"name":"Internacional"},{"id":18,"name":"Argentina"}],
  "competitions": [
    {"id":215,"countryId":1,"sportId":3,"name":"Wimbledon - Hombres","nameForURL":"wimbledon---men"},
    {"id":230,"countryId":1,"sportId":3,"name":"US Open - Hombres","nameForURL":"us-open---men"},
    {"id":702,"countryId":18,"sportId":2,"name":"Liga Nacional","nameForURL":"liga-nacional"},
    {"id":7,"countryId":18,"sportId":1,"name":"Liga Profesional","nameForURL":"liga-profesional"}
  ]
}`

func newCompetitionsServer(t *testing.T, body string) (*httptest.Server, func() string) {
	t.Helper()
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return srv, func() string { return lastQuery }
}

func TestDiscovery_UpsertsCompetitions(t *testing.T) {
	srv, lastQuery := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	res, err := DiscoverySports(context.Background(), repo, client, []string{"tennis", "basketball"}, nil)
	if err != nil {
		t.Fatalf("DiscoverySports: %v", err)
	}
	// Football (sportId 1) is neither requested nor enrolled, even
	// though the fixture carries it.
	if res.Upserted != 3 {
		t.Errorf("Upserted = %d, want 3", res.Upserted)
	}
	if res.TotalSeen != 3 {
		t.Errorf("TotalSeen = %d, want 3 (football filtered out)", res.TotalSeen)
	}
	if res.Source != "scores365" {
		t.Errorf("Source = %q, want scores365", res.Source)
	}
	// Without ?sports= the endpoint only returns a short "popular"
	// list, so the filter must reach the wire.
	if q := lastQuery(); !strings.Contains(q, "sports=") {
		t.Errorf("request query %q missing sports filter", q)
	}

	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for _, r := range rows {
		if r.Source != "scores365" {
			t.Errorf("row source = %q, want scores365", r.Source)
		}
		if !r.Enabled {
			t.Errorf("new rows must default to enabled=true (%s)", r.SourceLeagueId)
		}
		if r.Sport == "football" || r.Sport == "unknown" {
			t.Errorf("unexpected sport %q on row %s", r.Sport, r.SourceLeagueId)
		}
		// Comp IDs are namespaced through LeagueIDPrefix so a
		// 365scores comp ID cannot collide with a FotMob league ID
		// on the tournaments primary key.
		if !strings.HasPrefix(r.SourceLeagueId, "6000000") {
			t.Errorf("source_league_id %q is not LeagueIDPrefix-namespaced", r.SourceLeagueId)
		}
	}
}

func TestDiscovery_IsIdempotent(t *testing.T) {
	srv, _ := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	if _, err := DiscoverySports(context.Background(), repo, client, []string{"tennis"}, nil); err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := DiscoverySports(context.Background(), repo, client, []string{"tennis"}, nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Upserted != 0 {
		t.Errorf("second run Upserted = %d, want 0", second.Upserted)
	}
	if second.Unchanged != 2 {
		t.Errorf("second run Unchanged = %d, want 2", second.Unchanged)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("dedup failed, got %d rows, want 2", len(rows))
	}
}

// scraper_leagues.country is a size:8 column but the competitions API
// only publishes full country names, so EnsureLeague truncates rather
// than letting a strict-mode INSERT abort the whole sweep.
func TestDiscovery_TruncatesLongCountry(t *testing.T) {
	srv, _ := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	if _, err := DiscoverySports(context.Background(), repo, client, []string{"basketball"}, nil); err != nil {
		t.Fatalf("DiscoverySports: %v", err)
	}
	rows, _, err := repo.List(context.Background(), ListFilters{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if got := rows[0].Country; got != "Argentin" {
		t.Errorf("country = %q, want %q (truncated to 8 runes)", got, "Argentin")
	}
}

// An unknown slug must be dropped with a warning rather than failing
// the sweep; a list with no known slug at all is a no-op.
func TestDiscovery_UnknownSportSlugs(t *testing.T) {
	srv, lastQuery := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	res, err := DiscoverySports(context.Background(), repo, client, []string{"quidditch", "tennis"}, nil)
	if err != nil {
		t.Fatalf("DiscoverySports: %v", err)
	}
	if res.Upserted != 2 {
		t.Errorf("Upserted = %d, want 2 (unknown slug dropped, tennis kept)", res.Upserted)
	}

	res, err = DiscoverySports(context.Background(), repo, client, []string{"quidditch"}, nil)
	if err != nil {
		t.Fatalf("DiscoverySports (all unknown): %v", err)
	}
	if res.TotalSeen != 0 || res.Upserted != 0 {
		t.Errorf("all-unknown sweep = %+v, want a no-op", res)
	}
	if q := lastQuery(); strings.Contains(q, "quidditch") {
		t.Errorf("unknown slug leaked into the request query %q", q)
	}
}

func TestDiscovery_FetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	db := newDiscoveryDB(t)
	repo := NewRepository(db)
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	res, err := DiscoverySports(context.Background(), repo, client, []string{"tennis"}, nil)
	if err == nil {
		t.Fatal("want an error when the competitions endpoint fails")
	}
	if res.Upserted != 0 {
		t.Errorf("Upserted = %d, want 0 on a failed fetch", res.Upserted)
	}
}

func TestDiscovery_DefaultSlugsExcludeFootball(t *testing.T) {
	for _, slug := range DefaultSportSlugs {
		if slug == "football" {
			t.Fatal("DefaultSportSlugs must not enrol football; FotMob owns it")
		}
	}
	if len(DefaultSportSlugs) == 0 {
		t.Fatal("DefaultSportSlugs is empty")
	}
}
