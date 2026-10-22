//go:build integration

package catalog

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	if err := db.AutoMigrate(&ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// TestRepository_EnsureLeague_MariaDBInsertConflictReportsNotCreated
// pins the insert race against the production database.
//
// The conflicting insert is turned into a no-op by the unique index,
// and "no row was written" has to come back as a zero RowsAffected.
// MariaDB and SQLite report that differently, so the behaviour is
// verified on both.
func TestRepository_EnsureLeague_MariaDBInsertConflictReportsNotCreated(t *testing.T) {
	db := openCatalogTestDB(t)
	repo := NewRepository(db)

	source := fmt.Sprintf("itest-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Unscoped().Delete(&ScraperLeague{}, "source = ?", source)
	})

	ref := scraper.LeagueRef{Source: source, Name: "Conflict", Sport: "football", Country: "GB"}
	if _, err := repo.EnsureLeague(context.Background(), "itest", ref); err != nil {
		t.Fatalf("EnsureLeague: %v", err)
	}
	if err := db.Delete(&ScraperLeague{}, "source = ? AND source_league_id = ?", source, "itest").Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	created, err := repo.EnsureLeague(context.Background(), "itest", ref)
	if err != nil {
		t.Fatalf("EnsureLeague after soft delete: %v", err)
	}
	if created {
		t.Errorf("EnsureLeague must return created=false when the insert conflicts")
	}

	var live int64
	if err := db.Model(&ScraperLeague{}).Where("source = ?", source).Count(&live).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 0 {
		t.Errorf("the conflicting insert wrote a row: %d live rows", live)
	}
}
