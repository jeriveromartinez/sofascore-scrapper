//go:build integration

package events

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openMariaDB(t *testing.T) *gorm.DB {
	t.Helper()
	env := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
		env("DB_USER", "root"), os.Getenv("DB_PASSWORD"), env("DB_HOST", "localhost"),
		env("DB_PORT", "3306"), env("DB_NAME", "scores365"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql database: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	return db
}

func TestReconcileTeamLogosUsesSingleBulkUpdate_MariaDB(t *testing.T) {
	db := openMariaDB(t)
	if err := db.AutoMigrate(&Team{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ids := [3]int64{9_100_000_001, 9_100_000_002, 9_100_000_003}
	cleanup := func() { db.Unscoped().Where("team_id IN ?", ids[:]).Delete(&Team{}) }
	cleanup()
	t.Cleanup(cleanup)

	assertReconcileTeamLogos(t, db, ids)
}
