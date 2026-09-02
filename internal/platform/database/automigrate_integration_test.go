//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/database"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/seeder"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsnFromEnv()), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	return db
}

func columnExists(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	var n int
	row := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		table, column)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("inspect column: %v", err)
	}
	return n > 0
}

func TestAutoMigrateAll_CreatesAllTables(t *testing.T) {
	db := openTestDB(t)
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	want := []string{
		"users", "refresh_tokens", "domains", "apk_versions",
		"apk_upload_publications", "tournaments", "global_tournament_configs",
		"content_stats", "playback_logs", "devices", "device_tournaments",
		"teams", "events", "push_messages", "push_message_targets",
		"scheduled_pushes", "scheduled_push_targets", "delivery_attempts",
		"crash_reports",
	}
	for _, name := range want {
		if !db.Migrator().HasTable(name) {
			t.Errorf("table %q not created", name)
		}
	}
}

func TestAutoMigrateAll_PushTablesHaveDeletedAt(t *testing.T) {
	db := openTestDB(t)
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	for _, table := range []string{"push_messages", "delivery_attempts", "scheduled_pushes"} {
		if !columnExists(t, db, table, "deleted_at") {
			t.Errorf("%s.deleted_at missing", table)
		}
	}
}

func TestAutoMigrateAll_ApkSemverColumns(t *testing.T) {
	db := openTestDB(t)
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("AutoMigrateAll: %v", err)
	}
	for _, col := range []string{"version_major", "version_minor", "version_patch"} {
		if !columnExists(t, db, "apk_versions", col) {
			t.Errorf("apk_versions.%s missing", col)
		}
	}
}

func TestAutoMigrateAll_Idempotent(t *testing.T) {
	db := openTestDB(t)
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := database.AutoMigrateAll(db); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestSeedDefaultAdmin_OnFreshDB(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec("DELETE FROM users").Error; err != nil {
		t.Fatalf("truncate users: %v", err)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var got users.User
	if err := db.Where("email = ?", seeder.DefaultAdminEmail).First(&got).Error; err != nil {
		t.Fatalf("admin not found: %v", err)
	}
	if got.Role != users.RoleAdmin {
		t.Errorf("role = %q, want admin", got.Role)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(seeder.DefaultAdminPassword)); err != nil {
		t.Errorf("bcrypt compare: %v", err)
	}
}
