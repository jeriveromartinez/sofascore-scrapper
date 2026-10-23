// internal/scraper/catalog/migration_test.go
package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// legacyScraperLeague is the catalog table as it was built before the
// composite unique index: the index covers source_league_id alone, so
// the same upstream id cannot be used at two sources.
type legacyScraperLeague struct {
	gorm.Model
	Source         string `gorm:"column:source;size:32;not null;default:'fotmob';index:idx_scraper_leagues_source"`
	SourceLeagueId string `gorm:"column:source_league_id;size:64;not null;uniqueIndex:idx_scraper_leagues_source_league"`
	Name           string `gorm:"column:name;type:longtext;not null"`
	Country        string `gorm:"column:country;size:8;not null;default:''"`
	Sport          string `gorm:"column:sport;size:32;not null;default:'football'"`
	Enabled        bool   `gorm:"column:enabled;not null;index:idx_scraper_leagues_enabled"`
	OverrideSource *string
}

func (legacyScraperLeague) TableName() string { return "scraper_leagues" }

// legacyScraperLeagueLoose is the same table without any unique
// index, the shape a deployment that never got the index installed
// has. It is the state that can hold duplicate rows.
type legacyScraperLeagueLoose struct {
	gorm.Model
	Source         string `gorm:"column:source;size:32;not null;default:'fotmob';index:idx_scraper_leagues_source"`
	SourceLeagueId string `gorm:"column:source_league_id;size:64;not null;index:idx_scraper_leagues_source_league"`
	Name           string `gorm:"column:name;type:longtext;not null"`
	Country        string `gorm:"column:country;size:8;not null;default:''"`
	Sport          string `gorm:"column:sport;size:32;not null;default:'football'"`
	Enabled        bool   `gorm:"column:enabled;not null;index:idx_scraper_leagues_enabled"`
	OverrideSource *string
}

func (legacyScraperLeagueLoose) TableName() string { return "scraper_leagues" }

func openLegacyDB(t *testing.T, model any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(model); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	return db
}

// indexColumns returns the columns of the named index, or nil when the
// index does not exist.
func indexColumns(t *testing.T, db *gorm.DB, name string) ([]string, bool) {
	t.Helper()
	indexes, err := db.Migrator().GetIndexes(&ScraperLeague{})
	if err != nil {
		t.Fatalf("get indexes: %v", err)
	}
	for _, idx := range indexes {
		if idx.Name() == name {
			return idx.Columns(), true
		}
	}
	return nil, false
}

// TestRepository_Migration_ReplacesLegacyIndex is the regression test
// for the composite unique index: the legacy index covered
// source_league_id alone, so the same upstream number could not be
// used at two sources, and AutoMigrate never replaced it because the
// index name was unchanged.
func TestRepository_Migration_ReplacesLegacyIndex(t *testing.T) {
	db := openLegacyDB(t, &legacyScraperLeague{})

	cols, ok := indexColumns(t, db, sourceLeagueIndexName)
	if !ok {
		t.Fatal("the legacy schema is expected to carry the single-column index")
	}
	if len(cols) != 1 {
		t.Fatalf("legacy index columns = %v, want a single column", cols)
	}

	if err := MigrateSourceLeagueIndex(db); err != nil {
		t.Fatalf("migrate source league index: %v", err)
	}

	cols, ok = indexColumns(t, db, sourceLeagueIndexName)
	if !ok {
		t.Fatal("the composite index was not created")
	}
	if len(cols) != 2 || cols[0] != "source" || cols[1] != "source_league_id" {
		t.Errorf("index columns = %v, want [source source_league_id]", cols)
	}
}

// TestRepository_Migration_DeduplicatesAndAllowsSameIDPerSource is the
// regression test for the duplicate rows the new index would reject
// and for the per-source uniqueness the index is meant to express.
func TestRepository_Migration_DeduplicatesAndAllowsSameIDPerSource(t *testing.T) {
	db := openLegacyDB(t, &legacyScraperLeagueLoose{})

	rows := []legacyScraperLeagueLoose{
		{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Enabled: true},
		{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League (dup)", Enabled: true},
		{Source: "scores365", SourceLeagueId: "47", Name: "Other 47", Enabled: true},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed legacy row %d: %v", i, err)
		}
	}

	repo := NewRepository(db)
	if err := MigrateSourceLeagueIndex(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()

	var count int64
	if err := db.Model(&ScraperLeague{}).Where("source = ? AND source_league_id = ?", "fotmob", "47").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("the duplicate (fotmob, 47) rows were not removed, %d left", count)
	}

	if err := db.Model(&ScraperLeague{}).Where("source = ? AND source_league_id = ?", "scores365", "47").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("the (scores365, 47) row was removed, %d left", count)
	}

	// The same upstream id is now usable at another source.
	if err := repo.Create(ctx, &ScraperLeague{Source: "scores365", SourceLeagueId: "48", Name: "New", Enabled: true}); err != nil {
		t.Fatalf("create at a second source: %v", err)
	}

	// The same pair is still rejected.
	err := repo.Create(ctx, &ScraperLeague{Source: "fotmob", SourceLeagueId: "47", Name: "Again", Enabled: true})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("create of an existing (source, source_league_id) pair: got %v, want ErrDuplicate", err)
	}
}
