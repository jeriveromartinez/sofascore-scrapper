//go:build integration

package seeder

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper/catalog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// integrationDSN builds the MariaDB DSN from the environment used by
// the other integration tests.
func integrationDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
}

func newIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(integrationDSN()), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	return db
}

// TestSeedDefaults_ConcurrentFirstBoot is the regression test for the
// multi-backend first boot: every backend reconciles the curated
// league catalog at the same time. The reconciliation used to run in
// one big transaction that read and then wrote every curated row, so
// concurrent backends deadlocked on the gap locks of the rows they
// were about to insert and two of them exited.
func TestSeedDefaults_ConcurrentFirstBoot(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.Migrator().DropTable(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := db.AutoMigrate(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	const backends = 8
	start := make(chan struct{})
	errs := make([]error, backends)
	var wg sync.WaitGroup
	for i := range backends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = SeedDefaults(db, nil)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("backend %d: seed defaults: %v", i, err)
		}
	}

	var rows int64
	if err := db.Unscoped().Model(&catalog.ScraperLeague{}).Count(&rows).Error; err != nil {
		t.Fatalf("count leagues: %v", err)
	}
	if rows != int64(len(initialLeagues)) {
		t.Errorf("scraper_leagues rows = %d, want %d (one row per curated league)", rows, len(initialLeagues))
	}
}
