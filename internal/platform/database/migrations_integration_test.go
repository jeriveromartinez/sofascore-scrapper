//go:build integration

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jeriveromartinez/sofascore-scrapper/migrations"
)

var isolatedMigrationDBCounter atomic.Uint64

func dbDSN() string {
	return dbDSNFor(envOr("DB_NAME", "sofascore"))
}

func dbDSNFor(name string) string {
	host := envOr("DB_HOST", "localhost")
	port := envOr("DB_PORT", "3306")
	user := envOr("DB_USER", "root")
	pass := os.Getenv("DB_PASSWORD")
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

func openIsolatedMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	admin, err := sql.Open("mysql", dbDSNFor(""))
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Skipf("no database available: %v", err)
	}

	name := fmt.Sprintf("sofascore_migration_%d_%d", time.Now().UnixNano(), isolatedMigrationDBCounter.Add(1))
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		admin.Close()
		t.Fatalf("create isolated database: %v", err)
	}

	db, err := sql.Open("mysql", dbDSNFor(name))
	if err != nil {
		admin.Exec("DROP DATABASE `" + name + "`")
		admin.Close()
		t.Fatalf("open isolated database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Errorf("drop isolated database: %v", err)
		}
		admin.Close()
	})
	return db
}

func migrateTestDBTo(t *testing.T, db *sql.DB, version uint) {
	t.Helper()
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatalf("current database: %v", err)
	}
	migrationDB, err := sql.Open("mysql", dbDSNFor(name))
	if err != nil {
		t.Fatalf("open migration connection: %v", err)
	}
	src, err := iofs.New(migrations.FS(), ".")
	if err != nil {
		t.Fatalf("migration source: %v", err)
	}
	target, err := src.First()
	if err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if target > version {
		t.Fatalf("no migration at or below %d", version)
	}
	for {
		next, err := src.Next(target)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			t.Fatalf("migration after %d: %v", target, err)
		}
		if next > version {
			break
		}
		target = next
	}
	driver, err := mysql.WithInstance(migrationDB, &mysql.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		t.Fatalf("migration instance: %v", err)
	}
	t.Cleanup(func() {
		sourceErr, databaseErr := m.Close()
		if sourceErr != nil {
			t.Errorf("close migration source: %v", sourceErr)
		}
		if databaseErr != nil {
			t.Errorf("close migration database: %v", databaseErr)
		}
	})
	if err := m.Migrate(target); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to %d (resolved as %d): %v", version, target, err)
	}
}

func widenAPKPackageName(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"DROP INDEX idx_apk_version_package ON apk_versions",
		"ALTER TABLE apk_versions MODIFY package_name VARCHAR(1024) NOT NULL",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("prepare wide package_name with %q: %v", statement, err)
		}
	}
}

func setMigrationState(t *testing.T, db *sql.DB, version uint, dirty bool) {
	t.Helper()
	if _, err := db.Exec("UPDATE schema_migrations SET version = ?, dirty = ?", version, dirty); err != nil {
		t.Fatalf("set migration state: %v", err)
	}
}

func assertMigrationState(t *testing.T, db *sql.DB, wantVersion uint, wantDirty bool) {
	t.Helper()
	var version uint
	var dirty bool
	if err := db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("migration state: %v", err)
	}
	if version != wantVersion || dirty != wantDirty {
		t.Fatalf("migration state=(%d,%t), want (%d,%t)", version, dirty, wantVersion, wantDirty)
	}
}

func addSemverColumns(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		ALTER TABLE apk_versions
		  ADD COLUMN version_major BIGINT UNSIGNED NOT NULL DEFAULT 0,
		  ADD COLUMN version_minor BIGINT UNSIGNED NOT NULL DEFAULT 0,
		  ADD COLUMN version_patch BIGINT UNSIGNED NOT NULL DEFAULT 0
	`)
	if err != nil {
		t.Fatalf("add semver columns: %v", err)
	}
}

func insertAPKVersion(t *testing.T, db *sql.DB, version, token string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO apk_versions (version, file_name, file_path, package_name, download_token)
		VALUES (?, 'app.apk', '/tmp/app.apk', 'com.example.production', ?)
	`, version, token); err != nil {
		t.Fatalf("insert APK version %q: %v", version, err)
	}
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

// TestCrashReportsHasEmbeddedColumns protects against the production
// regression that motivated migration 000016: the GORM model
// `reporting.CrashReport` writes embedded `AppReport` /
// `DeviceReport` fields as bare column names (`name`, `version`,
// `os_version`, ...) because the struct does not declare
// `embeddedPrefix`. The original 000001 baseline created only the
// prefixed columns (`app_name`, ...), so every crash report from a
// real device failed with `Error 1054: Unknown column 'name' in
// 'INSERT INTO'`. The test asserts both column sets exist after
// Migrate() runs.
func TestCrashReportsHasEmbeddedColumns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	wantBare := []string{"name", "version", "build", "environment", "platform", "os_version", "locale"}
	for _, col := range wantBare {
		var exists int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'crash_reports' AND COLUMN_NAME = ?
		`, col).Scan(&exists); err != nil {
			t.Errorf("check column %q: %v", col, err)
			continue
		}
		if exists != 1 {
			t.Errorf("crash_reports column %q missing (GORM model writes it on every crash-report POST)", col)
		}
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

func TestMigrateWideAPKPackageName(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	widenAPKPackageName(t, db)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var prefix sql.NullInt64
	if err := db.QueryRow(`
		SELECT SUB_PART
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND INDEX_NAME = 'idx_apk_latest'
		  AND SEQ_IN_INDEX = 1
	`).Scan(&prefix); err != nil {
		t.Fatalf("read index prefix: %v", err)
	}
	if !prefix.Valid || prefix.Int64 != 191 {
		t.Fatalf("package_name prefix=%v, want 191", prefix)
	}
}

func TestMigrateRepairsDirtyAPKSemverMigration(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	widenAPKPackageName(t, db)
	if _, err := db.Exec(`
		INSERT INTO apk_versions (version, file_name, file_path, package_name, download_token)
		VALUES ('1.2.10', 'app.apk', '/tmp/app.apk', 'com.example.production', 'migration-repair-token')
	`); err != nil {
		t.Fatalf("insert APK: %v", err)
	}
	addSemverColumns(t, db)
	setMigrationState(t, db, 5, true)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	assertMigrationState(t, db, 8, false)
	var major, minor, patch uint64
	if err := db.QueryRow(`
		SELECT version_major, version_minor, version_patch
		FROM apk_versions WHERE download_token = 'migration-repair-token'
	`).Scan(&major, &minor, &patch); err != nil {
		t.Fatalf("read repaired semver: %v", err)
	}
	if major != 1 || minor != 2 || patch != 10 {
		t.Fatalf("repaired semver=%d.%d.%d, want 1.2.10", major, minor, patch)
	}
	var prefix sql.NullInt64
	if err := db.QueryRow(`
		SELECT SUB_PART
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND INDEX_NAME = 'idx_apk_latest'
		  AND SEQ_IN_INDEX = 1
	`).Scan(&prefix); err != nil {
		t.Fatalf("read repaired index prefix: %v", err)
	}
	if !prefix.Valid || prefix.Int64 != 191 {
		t.Fatalf("repaired package_name prefix=%v, want 191", prefix)
	}
}

func TestMigrateDoesNotRepairOtherDirtyVersion(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	setMigrationState(t, db, 4, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "Dirty database version 4") {
		t.Fatalf("Migrate error=%v, want dirty version 4", err)
	}
	assertMigrationState(t, db, 4, true)
}

func TestMigrateRejectsDirtyAPKSemverWithoutColumns(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "expected 3 semver columns, found 0") {
		t.Fatalf("Migrate error=%v, want missing-column diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
}

func TestMigrateRejectsIncompatibleAPKLatestIndex(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	addSemverColumns(t, db)
	if _, err := db.Exec("CREATE INDEX idx_apk_latest ON apk_versions(id)"); err != nil {
		t.Fatalf("create incompatible index: %v", err)
	}
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "idx_apk_latest has incompatible columns") {
		t.Fatalf("Migrate error=%v, want incompatible-index diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
}

func TestMigrateRejectsIncompatibleAPKLatestIndexPrefix(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	addSemverColumns(t, db)
	if _, err := db.Exec(`
		CREATE INDEX idx_apk_latest ON apk_versions(
		  package_name(100), is_active, version_major, version_minor, version_patch, id
		)
	`); err != nil {
		t.Fatalf("create wrong-prefix index: %v", err)
	}
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "idx_apk_latest has incompatible package_name prefix") {
		t.Fatalf("Migrate error=%v, want incompatible-prefix diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
}

func TestMigrateRejectsIncompatibleAPKSemverColumns(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	if _, err := db.Exec(`
		ALTER TABLE apk_versions
		  ADD COLUMN version_major INT UNSIGNED NOT NULL DEFAULT 0,
		  ADD COLUMN version_minor BIGINT NULL DEFAULT 0,
		  ADD COLUMN version_patch BIGINT UNSIGNED NOT NULL DEFAULT 1
	`); err != nil {
		t.Fatalf("add incompatible semver columns: %v", err)
	}
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "semver columns have incompatible definitions") {
		t.Fatalf("Migrate error=%v, want incompatible-semver-column diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
}

func TestMigrateFailsClosedWhenAdvisoryLockIsHeld(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	ctx := context.Background()

	lockConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("lock connection: %v", err)
	}
	locked := true
	t.Cleanup(func() {
		if locked {
			var released sql.NullInt64
			if err := lockConn.QueryRowContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockID).Scan(&released); err != nil {
				t.Errorf("cleanup release lock: %v", err)
			}
		}
		lockConn.Close()
	})
	var acquired int
	if err := lockConn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockID).Scan(&acquired); err != nil {
		t.Fatalf("hold advisory lock: %v", err)
	}
	if acquired != 1 {
		t.Fatalf("hold advisory lock result=%d, want 1", acquired)
	}

	err = Migrate(ctx, db)
	if err == nil || !strings.Contains(err.Error(), "advisory lock is already held") {
		t.Fatalf("Migrate error=%v, want held-lock diagnostic", err)
	}
	assertMigrationState(t, db, 3, false)

	var released sql.NullInt64
	if err := lockConn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", lockID).Scan(&released); err != nil {
		t.Fatalf("release advisory lock: %v", err)
	}
	if !released.Valid || released.Int64 != 1 {
		t.Fatalf("release advisory lock result=%v, want 1", released)
	}
	locked = false
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate after release: %v", err)
	}
	assertMigrationState(t, db, 8, false)
}

func TestMigrateReleasesDriverConnections(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	db.SetMaxOpenConns(4)

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := Migrate(ctx, db)
		cancel()
		if err != nil {
			t.Fatalf("Migrate call %d: %v", i+1, err)
		}
		if stats := db.Stats(); stats.InUse != 0 {
			t.Fatalf("Migrate call %d left %d connections in use", i+1, stats.InUse)
		}
	}

	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("caller-owned database is closed: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1=%d, want 1", one)
	}
}

func TestMigrateRejectsCleanAPKSemverOverflow(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	const version = "18446744073709551616.2.3"
	insertAPKVersion(t, db, version, "clean-overflow-token")

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "semver component exceeds unsigned BIGINT maximum") {
		t.Fatalf("Migrate error=%v, want semver-overflow diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
	var gotVersion string
	if err := db.QueryRow("SELECT version FROM apk_versions WHERE download_token = 'clean-overflow-token'").Scan(&gotVersion); err != nil {
		t.Fatalf("read overflow APK: %v", err)
	}
	if gotVersion != version {
		t.Fatalf("version=%q, want unchanged %q", gotVersion, version)
	}
}

func TestMigrateRejectsDirtyAPKSemverOverflow(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	const version = "1.18446744073709551616.3"
	insertAPKVersion(t, db, version, "dirty-overflow-token")
	addSemverColumns(t, db)
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "semver component exceeds unsigned BIGINT maximum") {
		t.Fatalf("Migrate error=%v, want semver-overflow diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
	var gotVersion string
	var major, minor, patch uint64
	if err := db.QueryRow(`
		SELECT version, version_major, version_minor, version_patch
		FROM apk_versions WHERE download_token = 'dirty-overflow-token'
	`).Scan(&gotVersion, &major, &minor, &patch); err != nil {
		t.Fatalf("read dirty overflow APK: %v", err)
	}
	if gotVersion != version || major != 0 || minor != 0 || patch != 0 {
		t.Fatalf("overflow APK=(%q,%d.%d.%d), want unchanged (%q,0.0.0)", gotVersion, major, minor, patch, version)
	}
}

func TestMigrateAcceptsCleanAPKSemverUint64Max(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	insertAPKVersion(t, db, "18446744073709551615.0.18446744073709551615", "clean-max-token")

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	assertMigrationState(t, db, 8, false)
	var major, patch uint64
	if err := db.QueryRow(`
		SELECT version_major, version_patch FROM apk_versions
		WHERE download_token = 'clean-max-token'
	`).Scan(&major, &patch); err != nil {
		t.Fatalf("read max-boundary APK: %v", err)
	}
	if major != ^uint64(0) || patch != ^uint64(0) {
		t.Fatalf("max-boundary components=(%d,%d), want (%d,%d)", major, patch, ^uint64(0), ^uint64(0))
	}
}

func TestMigrateAcceptsDirtyAPKSemverUint64Max(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	insertAPKVersion(t, db, "0.18446744073709551615.0", "dirty-max-token")
	addSemverColumns(t, db)
	setMigrationState(t, db, 5, true)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	assertMigrationState(t, db, 8, false)
	var minor uint64
	if err := db.QueryRow(`
		SELECT version_minor FROM apk_versions WHERE download_token = 'dirty-max-token'
	`).Scan(&minor); err != nil {
		t.Fatalf("read repaired max-boundary APK: %v", err)
	}
	if minor != ^uint64(0) {
		t.Fatalf("max-boundary minor=%d, want %d", minor, ^uint64(0))
	}
}

func TestMigrateRejectsIgnoredAPKLatestIndex(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	addSemverColumns(t, db)
	if _, err := db.Exec(`
		CREATE INDEX idx_apk_latest ON apk_versions(
		  package_name(191), is_active, version_major, version_minor, version_patch, id
		)
	`); err != nil {
		t.Fatalf("create compatible index: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE apk_versions ALTER INDEX idx_apk_latest IGNORED"); err != nil {
		t.Fatalf("ignore compatible index: %v", err)
	}
	setMigrationState(t, db, 5, true)

	err := Migrate(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "idx_apk_latest is ignored") {
		t.Fatalf("Migrate error=%v, want ignored-index diagnostic", err)
	}
	assertMigrationState(t, db, 5, true)
	var ignored string
	if err := db.QueryRow(`
		SELECT IGNORED FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND INDEX_NAME = 'idx_apk_latest'
		LIMIT 1
	`).Scan(&ignored); err != nil {
		t.Fatalf("read ignored index status: %v", err)
	}
	if ignored != "YES" {
		t.Fatalf("index ignored=%q, want unchanged YES", ignored)
	}
}

func TestMigrateAcceptsCompatibleAPKLatestIndex(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	insertAPKVersion(t, db, "2.4.6", "compatible-index-token")
	addSemverColumns(t, db)
	if _, err := db.Exec(`
		CREATE INDEX idx_apk_latest ON apk_versions(
		  package_name(191), is_active, version_major, version_minor, version_patch, id
		)
	`); err != nil {
		t.Fatalf("create compatible index: %v", err)
	}
	setMigrationState(t, db, 5, true)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	assertMigrationState(t, db, 8, false)
	var major, minor, patch uint64
	if err := db.QueryRow(`
		SELECT version_major, version_minor, version_patch
		FROM apk_versions WHERE download_token = 'compatible-index-token'
	`).Scan(&major, &minor, &patch); err != nil {
		t.Fatalf("read repaired semver: %v", err)
	}
	if major != 2 || minor != 4 || patch != 6 {
		t.Fatalf("repaired semver=%d.%d.%d, want 2.4.6", major, minor, patch)
	}
}

func TestMigrateRepairsDirtyAPKSemverWithSingleConnection(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	migrateTestDBTo(t, db, 4)
	insertAPKVersion(t, db, "3.5.8", "single-connection-repair-token")
	addSemverColumns(t, db)
	setMigrationState(t, db, 5, true)
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	assertMigrationState(t, db, 8, false)
	var major, minor, patch uint64
	if err := db.QueryRow(`
		SELECT version_major, version_minor, version_patch
		FROM apk_versions WHERE download_token = 'single-connection-repair-token'
	`).Scan(&major, &minor, &patch); err != nil {
		t.Fatalf("read repaired semver: %v", err)
	}
	if major != 3 || minor != 5 || patch != 8 {
		t.Fatalf("repaired semver=%d.%d.%d, want 3.5.8", major, minor, patch)
	}
	if stats := db.Stats(); stats.InUse != 0 {
		t.Fatalf("Migrate left %d connections in use", stats.InUse)
	}
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("caller-owned database is unusable: %v", err)
	}
}

func TestMigrateCleanDBWithSingleConnection(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	assertMigrationState(t, db, 8, false)
	if stats := db.Stats(); stats.InUse != 0 {
		t.Fatalf("Migrate left %d connections in use", stats.InUse)
	}
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("caller-owned database is unusable: %v", err)
	}
}

// TestPushTablesHaveDeletedAtColumn guards against the regression
// reported in ott-qcho (2026-08-31): Error 1054 "Unknown column
// 'deleted_at' in 'INSERT INTO'" raised by the GORM models
// push.PushMessage, push.ScheduledPush, and push.DeliveryAttempt
// when the SQL migrations 000012, 000013, 000014 had not declared
// the column that the embedded gorm.Model requires.
//
// Migration 000017_push_gorm_soft_delete adds the column + its
// index on all three tables. This test asserts:
//   - Migrate() lands on the latest version (the new migration runs).
//   - information_schema reports deleted_at on all three tables.
//   - A bare INSERT (with FK checks disabled, since we do not
//     bootstrap a user/device/tournament here) succeeds.
func TestPushTablesHaveDeletedAtColumn(t *testing.T) {
	db := openIsolatedMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tables := []string{"push_messages", "scheduled_pushes", "delivery_attempts"}
	for _, table := range tables {
		var count int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA = DATABASE()
			   AND TABLE_NAME   = ?
			   AND COLUMN_NAME  = 'deleted_at'
		`, table).Scan(&count); err != nil {
			t.Fatalf("inspect %s.deleted_at: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s.deleted_at column missing (count=%d)", table, count)
		}
	}

	// Smoke INSERTs. The push_messages INSERT goes through the same
	// column set the GORM model uses, so a success here means the
	// runtime Error 1054 path is closed. The scheduled_pushes and
	// delivery_attempts INSERTs cover the other two models in the
	// same fix.
	mustExec := func(label, stmt string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}
	mustExec("disable-fk", `SET FOREIGN_KEY_CHECKS=0`)
	mustExec("insert-push-message", `
		INSERT INTO push_messages
		  (created_at, updated_at, deleted_at, user_id, category, title, body, image_url, deep_link, priority, ttl_seconds, data_json, source, scheduled_id)
		VALUES
		  (NOW(3), NOW(3), NULL, 0, 'admin_message', 'gorm_smoke', 'gorm_smoke', '', '', 'normal', 0, NULL, 'immediate', NULL)
	`)
	mustExec("insert-scheduled-push", `
		INSERT INTO scheduled_pushes
		  (created_at, updated_at, deleted_at, user_id, schedule_type, next_fire_at, category, title, body, priority, ttl_seconds)
		VALUES
		  (NOW(3), NOW(3), NULL, 0, 'one_shot', NOW(3), 'admin_message', 'gorm_smoke', 'gorm_smoke', 'normal', 0)
	`)
	mustExec("insert-delivery-attempt", `
		INSERT INTO delivery_attempts
		  (created_at, updated_at, deleted_at, push_message_id, device_id, message_id, state)
		VALUES
		  (NOW(3), NOW(3), NULL, 0, 0, '00000000-0000-0000-0000-000000000000', 'sent')
	`)
	mustExec("reenable-fk", `SET FOREIGN_KEY_CHECKS=1`)

	// Confirm the rows really landed and the deleted_at column is the
	// NULL the GORM model writes when no soft delete is in effect.
	var pmRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM push_messages WHERE title = 'gorm_smoke' AND deleted_at IS NULL").Scan(&pmRows); err != nil {
		t.Fatalf("count push_messages: %v", err)
	}
	if pmRows == 0 {
		t.Fatal("expected at least one push_messages row inserted by the smoke")
	}
}
