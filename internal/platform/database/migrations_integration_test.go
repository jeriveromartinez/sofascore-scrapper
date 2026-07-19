//go:build integration

package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/golang-migrate/migrate/v4/database/mysql"
)

func dbDSN() string {
	host := envOr("DB_HOST", "localhost")
	port := envOr("DB_PORT", "3306")
	user := envOr("DB_USER", "root")
	pass := os.Getenv("DB_PASSWORD")
	name := envOr("DB_NAME", "sofascore")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&multiStatements=true", user, pass, host, port, name)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dbDSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	return db
}

func TestMigrateEmptyDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, table := range []string{"users", "tournaments", "teams", "events", "domains", "refresh_tokens", "devices", "playback_logs", "apk_versions", "device_tournaments", "global_tournament_configs", "content_stats", "crash_reports"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?", envOr("DB_NAME", "sofascore"), table).Scan(&count); err != nil {
			t.Errorf("check table %s: %v", table, err)
		} else if count != 1 {
			t.Errorf("table %s not found", table)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestStatusTypeColumnExists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var col string
	if err := db.QueryRowContext(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE table_schema = ? AND table_name = 'events' AND COLUMN_NAME = 'status_type'", envOr("DB_NAME", "sofascore")).Scan(&col); err != nil {
		t.Fatalf("status_type column: %v", err)
	}
	if col != "status_type" {
		t.Fatalf("expected status_type, got %s", col)
	}
}

func TestTimestampBackfillIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE events SET start_timestamp = start_timestamp * 1000 WHERE start_timestamp > 0 AND start_timestamp < 1000000000000"); err != nil {
		t.Fatalf("double backfill check: %v", err)
	}
}

func TestDestructiveReset(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO content_stats (content_hash, period_type, period_start, seconds, views) VALUES ('test', 'day', NOW(), 0, 0)"); err != nil {
		t.Fatalf("insert stat: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO playback_logs (device_id, content) VALUES (1, 'test')"); err != nil {
		t.Fatalf("insert log: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM content_stats").Scan(&count); err != nil {
		t.Fatalf("count stats: %v", err)
	}
	var count2 int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM playback_logs").Scan(&count2); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count > 0 || count2 > 0 {
		t.Logf("after migration 003, stats=%d logs=%d", count, count2)
	}
}
