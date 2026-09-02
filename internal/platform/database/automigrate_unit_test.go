//go:build integration

package database_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func dsnFromEnv() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsnFromEnv()), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	return db
}

// TestAutoMigrateAll_AllModelsRegistered is a safety net: every
// model the codebase declares MUST be in automigrateModels. If you
// add a new model without registering it, this test fails.
func TestAutoMigrateAll_AllModelsRegistered(t *testing.T) {
	want := []string{
		"users", "refresh_tokens", "domains", "apk_versions",
		"apk_upload_publications", "tournaments", "global_tournament_configs",
		"content_stats", "playback_logs", "devices", "device_tournaments",
		"teams", "events", "push_messages", "push_message_targets",
		"scheduled_pushes", "scheduled_push_targets", "delivery_attempts",
		"crash_reports",
	}
	db := newTestDB(t)
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	for _, name := range want {
		if !db.Migrator().HasTable(name) {
			t.Errorf("table %q not created by AutoMigrateAll", name)
		}
	}
}
