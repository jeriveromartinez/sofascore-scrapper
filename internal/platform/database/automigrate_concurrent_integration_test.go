//go:build integration

package database_test

import (
	"sync"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// concurrentMigrateDatabase is the scratch database the concurrent
// migration test migrates. It is created and dropped by the test, so
// it never touches the database the rest of the suite uses.
const concurrentMigrateDatabase = "scores365_concurrent_migrate"

// TestAutoMigrateAll_ConcurrentBackends covers the deployment that
// runs one backend per instance: they all boot against the same
// MariaDB and every one of them migrates the schema.
//
// GORM's AutoMigrate asks the server whether an index exists and
// creates it when the answer is no, so two backends that migrate at
// the same time can both see "no" and both create it. The loser used
// to abort its boot with a duplicate key error.
//
// The test migrates twice: the first round starts from an empty
// schema (a first deploy), the second one from the schema the first
// round left behind (every deploy after that).
func TestAutoMigrateAll_ConcurrentBackends(t *testing.T) {
	admin := openDatabase(t, dsnFromEnv())
	dropDatabase(t, admin, concurrentMigrateDatabase)
	createDatabase(t, admin, concurrentMigrateDatabase)
	t.Cleanup(func() { dropDatabase(t, admin, concurrentMigrateDatabase) })

	migrateConcurrently(t, "an empty schema")
	migrateConcurrently(t, "an existing schema")
}

// migrateConcurrently runs AutoMigrateAll from several backends at
// once and fails when one of them does not come through.
func migrateConcurrently(t *testing.T, schema string) {
	t.Helper()

	const instances = 3
	start := make(chan struct{})
	results := make(chan error, instances)
	var wg sync.WaitGroup
	for range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := gorm.Open(mysql.Open(dsnForDatabase(concurrentMigrateDatabase)), &gorm.Config{})
			if err != nil {
				results <- err
				return
			}
			<-start
			results <- database.AutoMigrateAll(db)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent AutoMigrateAll over %s: %v", schema, err)
		}
	}
}

func openDatabase(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	return db
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
