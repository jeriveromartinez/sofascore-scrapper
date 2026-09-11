// internal/scraper/catalog/repository_test.go
package catalog

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
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
