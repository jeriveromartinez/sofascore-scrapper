// internal/scraper/catalog/repository_test.go
package catalog

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
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

func TestRepository_Create(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	sl := &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "GB", Sport: "football", Enabled: true}
	if err := repo.Create(context.Background(), sl); err != nil {
		t.Fatalf("create: %v", err)
	}
	if sl.ID == 0 {
		t.Fatalf("ID not set")
	}
}

func TestRepository_Create_DuplicateUniqueErrors(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	sl1 := &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "L1", Enabled: true}
	if err := repo.Create(context.Background(), sl1); err != nil {
		t.Fatalf("first: %v", err)
	}
	sl2 := &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "L2", Enabled: true}
	err := repo.Create(context.Background(), sl2)
	if err == nil {
		t.Fatalf("expected unique violation")
	}
}

func TestRepository_ActiveLeagues(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "PL", Enabled: true})
	repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: "87", Name: "LL", Enabled: false})
	leagues, err := repo.ActiveLeagues(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(leagues) != 1 || leagues[0].SourceLeagueId != "47" {
		t.Fatalf("leagues: %+v", leagues)
	}
}

func TestRepository_SoftDelete(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	sl := &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "PL", Enabled: true}
	repo.Create(context.Background(), sl)
	if err := repo.SoftDelete(context.Background(), sl.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := repo.GetByID(context.Background(), sl.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after soft delete, got %+v", got)
	}
}

func TestRepository_SearchLocalByName(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "GB", Sport: "football", Enabled: true})
	repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: "87", Name: "LaLiga", Country: "ES", Sport: "football", Enabled: true})
	repo.Create(context.Background(), &ScraperLeague{Source: "fotmob", SourceLeagueId: "54", Name: "Bundesliga", Country: "DE", Sport: "football", Enabled: true})

	// Empty query short-circuits.
	if got, err := repo.SearchLocalByName(context.Background(), "   "); err != nil || got != nil {
		t.Errorf("empty query: got=%v err=%v, want both nil", got, err)
	}

	// Case-insensitive substring match.
	got, err := repo.SearchLocalByName(context.Background(), "PREMIER")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 || got[0].SourceLeagueId != "47" {
		t.Errorf("want only Premier League, got %+v", got)
	}

	// Substring in the middle.
	got, _ = repo.SearchLocalByName(context.Background(), "iga")
	if len(got) != 2 {
		t.Errorf("want LaLiga+Bundesliga, got %d rows: %+v", len(got), got)
	}

	// No match.
	got, _ = repo.SearchLocalByName(context.Background(), "NBA")
	if len(got) != 0 {
		t.Errorf("want 0 rows for NBA, got %d", len(got))
	}
}

func TestRepository_ActiveLeaguesBySource_RoutesPinnedRow(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	// Row 1: scores365 default — scores365 dispatcher sees it.
	mustCreate(t, repo, &ScraperLeague{Source: "scores365", SourceLeagueId: "pin-1", Name: "DefaultScores365", Enabled: true})
	// Row 2: source=scores365 but pinned AWAY to fotmob — scores365 must NOT see it; fotmob must.
	mustCreate(t, repo, &ScraperLeague{Source: "scores365", SourceLeagueId: "pin-2", Name: "PinnedAway", Enabled: true, OverrideSource: strPtr("fotmob")})
	// Row 3: source=fotmob but pinned TO scores365 — scores365 must see it (pinned in).
	mustCreate(t, repo, &ScraperLeague{Source: "fotmob", SourceLeagueId: "pin-3", Name: "PinnedIn", Enabled: true, OverrideSource: strPtr("scores365")})

	gotS365, err := repo.ActiveLeaguesBySource(ctx, "scores365")
	if err != nil {
		t.Fatalf("scores365: %v", err)
	}
	if len(gotS365) != 2 {
		t.Fatalf("scores365: want 2 rows (pin-1, pin-3), got %d: %+v", len(gotS365), gotS365)
	}
	ids := map[string]bool{}
	for _, r := range gotS365 {
		ids[r.SourceLeagueId] = true
	}
	if !ids["pin-1"] || !ids["pin-3"] {
		t.Errorf("scores365: want pin-1 and pin-3, got %+v", gotS365)
	}

	gotFM, err := repo.ActiveLeaguesBySource(ctx, "fotmob")
	if err != nil {
		t.Fatalf("fotmob: %v", err)
	}
	if len(gotFM) != 1 || gotFM[0].SourceLeagueId != "pin-2" {
		t.Fatalf("fotmob: want only pin-2, got %+v", gotFM)
	}
}

func TestRepository_ActiveLeaguesBySource_DisabledExcluded(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	mustCreate(t, repo, &ScraperLeague{Source: "scores365", SourceLeagueId: "dis-1", Name: "Active", Enabled: true})
	mustCreate(t, repo, &ScraperLeague{Source: "scores365", SourceLeagueId: "dis-2", Name: "Off", Enabled: false})
	mustCreate(t, repo, &ScraperLeague{Source: "fotmob", SourceLeagueId: "dis-3", Name: "OffFM", Enabled: false, OverrideSource: strPtr("scores365")})

	for _, src := range []string{"scores365", "fotmob"} {
		got, err := repo.ActiveLeaguesBySource(ctx, src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		// None of the disabled source_league_ids may appear.
		for _, r := range got {
			if r.SourceLeagueId == "dis-2" || r.SourceLeagueId == "dis-3" {
				t.Errorf("%s dispatcher leaked disabled row %s", src, r.SourceLeagueId)
			}
		}
		// Specifically: only dis-1 (enabled) is visible at all.
		if src == "scores365" {
			if len(got) != 1 || got[0].SourceLeagueId != "dis-1" {
				t.Errorf("scores365: want only dis-1, got %+v", got)
			}
		} else {
			if len(got) != 0 {
				t.Errorf("fotmob: want 0 (no enabled row has effective source fotmob), got %+v", got)
			}
		}
	}
}

func mustCreate(t *testing.T, repo *Repository, sl *ScraperLeague) {
	t.Helper()
	if err := repo.Create(context.Background(), sl); err != nil {
		t.Fatalf("create %+v: %v", sl, err)
	}
}

func strPtr(s string) *string { return &s }

// TestRepository_EnsureLeague_FirstCallReturnsCreated covers Fix 4.
// EnsureLeague now returns (created bool, err error): created=true
// when a row was inserted, created=false when the row already
// existed. The dispatch loop relies on this to skip upserts for
// leagues the operator has disabled on a previous tick.
func TestRepository_EnsureLeague_FirstCallReturnsCreated(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	created, err := repo.EnsureLeague(context.Background(), "scores365", scraper.LeagueRef{
		Source: "scores365", Name: "MLB", Sport: "baseball", Country: "USA",
	})
	if err != nil {
		t.Fatalf("EnsureLeague: %v", err)
	}
	if !created {
		t.Errorf("first EnsureLeague must return created=true, got false")
	}
}

func TestRepository_EnsureLeague_SecondCallReturnsNotCreated(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ref := scraper.LeagueRef{Source: "scores365", Name: "MLB", Sport: "baseball", Country: "USA"}
	if _, err := repo.EnsureLeague(context.Background(), "scores365", ref); err != nil {
		t.Fatalf("first EnsureLeague: %v", err)
	}
	created, err := repo.EnsureLeague(context.Background(), "scores365", ref)
	if err != nil {
		t.Fatalf("second EnsureLeague: %v", err)
	}
	if created {
		t.Errorf("second EnsureLeague must return created=false, got true")
	}
}

// TestRepository_EnsureLeague_PreservesDisabledRow is the
// regression for Fix 4. When an operator disables a scores365
// league via the admin UI, EnsureLeague on a subsequent dispatch
// tick must report created=false so the loop knows to skip the
// upsert (otherwise the disabled league would silently start
// receiving matches again, defeating the opt-out).
func TestRepository_EnsureLeague_PreservesDisabledRow(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ref := scraper.LeagueRef{Source: "scores365", Name: "MLB", Sport: "baseball", Country: "USA"}
	if _, err := repo.EnsureLeague(context.Background(), "scores365", ref); err != nil {
		t.Fatalf("EnsureLeague: %v", err)
	}
	// Operator disables the row.
	if err := db.Model(&ScraperLeague{}).Where("source = ? AND source_league_id = ?", "scores365", "scores365").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}
	created, err := repo.EnsureLeague(context.Background(), "scores365", ref)
	if err != nil {
		t.Fatalf("EnsureLeague: %v", err)
	}
	if created {
		t.Errorf("EnsureLeague must return created=false for a pre-existing (disabled) row")
	}
	// Row stays disabled.
	var row ScraperLeague
	if err := db.Where("source = ? AND source_league_id = ?", "scores365", "scores365").First(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.Enabled {
		t.Errorf("EnsureLeague must NOT flip an existing disabled row back to enabled")
	}
}

func TestRepository_SearchLocalByName_IgnoresSoftDeleted(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	live := &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Country: "GB", Sport: "football", Enabled: true}
	deleted := &ScraperLeague{Source: "fotmob", SourceLeagueId: "87", Name: "Premier League", Country: "ES", Sport: "football", Enabled: true}
	repo.Create(context.Background(), live)
	repo.Create(context.Background(), deleted)
	repo.SoftDelete(context.Background(), deleted.ID)

	got, _ := repo.SearchLocalByName(context.Background(), "Premier")
	if len(got) != 1 || got[0].SourceLeagueId != "47" {
		t.Errorf("want only live Premier League (47), got %+v", got)
	}
}
