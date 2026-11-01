//go:build integration

package database_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/database"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper/catalog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	concurrentMigrateInstances = 3
	sourceLeagueIndexName      = "idx_scraper_leagues_source_league"
)

// legacyScraperLeague is the catalog table as it was built before the
// composite unique index: the index that carries the final name is
// non-unique and covers source_league_id alone. AutoMigrate skips it
// because the name matches, and MigrateSourceLeagueIndex replaces it.
type legacyScraperLeague struct {
	gorm.Model
	Source         string  `gorm:"column:source;size:32;not null;default:'fotmob';index:idx_scraper_leagues_source"`
	SourceLeagueId string  `gorm:"column:source_league_id;size:64;not null;index:idx_scraper_leagues_source_league"`
	Name           string  `gorm:"column:name;type:longtext;not null"`
	Country        string  `gorm:"column:country;size:8;not null;default:''"`
	Sport          string  `gorm:"column:sport;size:32;not null;default:'football'"`
	Enabled        bool    `gorm:"column:enabled;not null;index:idx_scraper_leagues_enabled"`
	OverrideSource *string `gorm:"column:override_source;size:32"`
}

func (legacyScraperLeague) TableName() string { return "scraper_leagues" }

// TestAutoMigrateAll_ConcurrentBackends covers the deployment that
// runs one backend per instance: they all boot against the same
// MariaDB and every one of them migrates the schema.
//
// The test migrates three times: from an empty schema (a first
// deploy), from the schema the first round left behind (every deploy
// after that) and from the legacy catalog shape with duplicate rows
// (an upgrade from before the composite index). Every round asserts
// the schema the migration is supposed to leave behind, not only that
// the backends returned no error.
func TestAutoMigrateAll_ConcurrentBackends(t *testing.T) {
	name := uniqueDatabaseName()
	admin := openDatabase(t, dsnFromEnv())
	closeOnCleanup(t, admin)
	resetDatabase(t, admin, name)
	t.Cleanup(func() { dropDatabase(t, admin, name) })

	migrateConcurrently(t, name, "an empty schema")
	migrateConcurrently(t, name, "an existing schema")

	resetDatabase(t, admin, name)
	seedLegacyCatalog(t, name)
	migrateConcurrently(t, name, "a legacy catalog schema")
}

// migrateConcurrently runs AutoMigrateAll from several backends at
// once and checks the schema it leaves behind.
func migrateConcurrently(t *testing.T, name, schema string) {
	t.Helper()

	dbs := make([]*gorm.DB, 0, concurrentMigrateInstances)
	for range concurrentMigrateInstances {
		db, err := gorm.Open(mysql.Open(dsnForDatabase(name)), &gorm.Config{})
		if err != nil {
			t.Fatalf("open a backend against %s: %v", name, err)
		}
		dbs = append(dbs, db)
	}
	defer func() {
		for _, db := range dbs {
			closeDatabase(db)
		}
	}()

	start := make(chan struct{})
	results := make(chan error, len(dbs))
	var wg sync.WaitGroup
	for _, db := range dbs {
		wg.Add(1)
		go func(db *gorm.DB) {
			defer wg.Done()
			<-start
			results <- database.AutoMigrateAll(db)
		}(db)
	}
	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent AutoMigrateAll over %s: %v", schema, err)
		}
	}
	assertMigratedSchema(t, dbs[0], schema)
}

// assertMigratedSchema checks the post-conditions a green run must
// leave behind, so a run that "did not error" but corrupted the schema
// cannot pass.
func assertMigratedSchema(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	assertCompositeCatalogIndex(t, db, schema)
	assertNoDuplicateLeagues(t, db, schema)
}

func assertCompositeCatalogIndex(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	indexes, err := db.Migrator().GetIndexes(&catalog.ScraperLeague{})
	if err != nil {
		t.Fatalf("get indexes after %s: %v", schema, err)
	}
	for _, idx := range indexes {
		if idx.Name() != sourceLeagueIndexName {
			continue
		}
		cols := idx.Columns()
		if len(cols) != 2 || cols[0] != "source" || cols[1] != "source_league_id" {
			t.Fatalf("after %s the catalog index columns are %v, want [source source_league_id]", schema, cols)
		}
		unique, ok := idx.Unique()
		if !ok || !unique {
			t.Fatalf("after %s the catalog index is not unique", schema)
		}
		return
	}
	t.Fatalf("after %s the catalog composite index is missing", schema)
}

func assertNoDuplicateLeagues(t *testing.T, db *gorm.DB, schema string) {
	t.Helper()
	var duplicates []string
	err := db.Raw(
		"SELECT CONCAT(source, '-', source_league_id) FROM scraper_leagues " +
			"GROUP BY source, source_league_id HAVING COUNT(*) > 1",
	).Scan(&duplicates).Error
	if err != nil {
		t.Fatalf("count duplicate leagues after %s: %v", schema, err)
	}
	if len(duplicates) != 0 {
		t.Fatalf("after %s duplicate leagues remain: %v", schema, duplicates)
	}
}

// seedLegacyCatalog builds the catalog table the way a deployment
// that predates the composite index has it, duplicates included.
func seedLegacyCatalog(t *testing.T, name string) {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsnForDatabase(name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open %s to seed the legacy catalog: %v", name, err)
	}
	defer closeDatabase(db)

	if err := db.AutoMigrate(&legacyScraperLeague{}); err != nil {
		t.Fatalf("create the legacy catalog schema: %v", err)
	}
	rows := []legacyScraperLeague{
		{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League", Enabled: true},
		{Source: "fotmob", SourceLeagueId: "47", Name: "Premier League (dup)", Enabled: true},
		{Source: "scores365", SourceLeagueId: "47", Name: "Other 47", Enabled: true},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed legacy row %d: %v", i, err)
		}
	}
}

func uniqueDatabaseName() string {
	return fmt.Sprintf("scores365_mig_%d_%d", os.Getpid(), time.Now().UnixNano())
}

func openDatabase(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		if isUnreachable(err) {
			t.Skipf("mariadb not reachable, skipping integration test: %v", err)
		}
		t.Fatalf("open database: %v", err)
	}
	return db
}

// isUnreachable keeps a missing server a skip while a wrong password
// or a missing database fails the suite.
func isUnreachable(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"connection refused",
		"no such host",
		"i/o timeout",
		"connectex",
		"dial tcp",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func closeOnCleanup(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Cleanup(func() { closeDatabase(db) })
}

func closeDatabase(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func resetDatabase(t *testing.T, admin *gorm.DB, name string) {
	t.Helper()
	dropDatabase(t, admin, name)
	createDatabase(t, admin, name)
}

func createDatabase(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	if err := db.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
}

func dropDatabase(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	if err := db.Exec("DROP DATABASE IF EXISTS " + name).Error; err != nil {
		t.Fatalf("drop database %s: %v", name, err)
	}
}
