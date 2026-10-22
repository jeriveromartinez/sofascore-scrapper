//go:build integration

package database_test

import (
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/database"
)

// TestPreMigrateLegacyEvents_BackfillsDistinctValuesOnMySQL covers the
// production dialect: MySQL and MariaDB parse "||" as logical OR
// unless PIPES_AS_CONCAT is in sql_mode, so the previous
// "'legacy-' || id" backfill assigned the same value to every row and
// the unique index could not be created — the server refused to boot
// on any database with legacy events rows.
func TestPreMigrateLegacyEvents_BackfillsDistinctValuesOnMySQL(t *testing.T) {
	db := newTestDB(t)

	if err := db.Exec("DROP TABLE IF EXISTS events").Error; err != nil {
		t.Fatalf("drop events: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP TABLE IF EXISTS events").Error; err != nil {
			t.Logf("cleanup drop events: %v", err)
		}
	})

	if err := db.Exec(`CREATE TABLE events (
		id bigint unsigned NOT NULL AUTO_INCREMENT,
		source varchar(32) NOT NULL DEFAULT '',
		created_at datetime(3) NULL,
		updated_at datetime(3) NULL,
		deleted_at datetime(3) NULL,
		PRIMARY KEY (id)
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatalf("create legacy events table: %v", err)
	}
	if err := db.Exec("INSERT INTO events (source) VALUES ('sofascore'), ('fotmob'), ('sofascore')").Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	if err := database.PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	type row struct {
		ID              uint
		ExternalMatchID string
	}
	var rows []row
	if err := db.Raw("SELECT id, external_match_id FROM events ORDER BY id").Scan(&rows).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ExternalMatchID == "" {
			t.Errorf("row %d was not backfilled", r.ID)
			continue
		}
		if len(r.ExternalMatchID) < 7 || r.ExternalMatchID[:7] != "legacy-" {
			t.Errorf("row %d backfilled to %q, want a legacy- prefix", r.ID, r.ExternalMatchID)
		}
		if seen[r.ExternalMatchID] {
			t.Errorf("backfilled value %q is not unique", r.ExternalMatchID)
		}
		seen[r.ExternalMatchID] = true
	}

	// The unique index the migration adds is the step that used to
	// fail with a duplicate-key error.
	if err := db.Exec("ALTER TABLE events ADD UNIQUE INDEX idx_events_external_match_id (external_match_id)").Error; err != nil {
		t.Fatalf("create the unique index over the backfilled column: %v", err)
	}
}

// TestPreMigrateLegacyEvents_BackfillsEmptyColumnOnMySQL covers the
// other upgrade path: the column exists but every row holds the empty
// string. The previous "'legacy-' || id" backfill assigned the value
// 1 to every row on MySQL/MariaDB (|| is logical OR there), so the
// unique index creation failed with a duplicate-key error.
func TestPreMigrateLegacyEvents_BackfillsEmptyColumnOnMySQL(t *testing.T) {
	db := newTestDB(t)

	if err := db.Exec("DROP TABLE IF EXISTS events").Error; err != nil {
		t.Fatalf("drop events: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP TABLE IF EXISTS events").Error; err != nil {
			t.Logf("cleanup drop events: %v", err)
		}
	})

	if err := db.Exec(`CREATE TABLE events (
		id bigint unsigned NOT NULL AUTO_INCREMENT,
		external_match_id varchar(64) NOT NULL DEFAULT '',
		source varchar(32) NOT NULL DEFAULT '',
		created_at datetime(3) NULL,
		updated_at datetime(3) NULL,
		deleted_at datetime(3) NULL,
		PRIMARY KEY (id)
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatalf("create legacy events table: %v", err)
	}
	if err := db.Exec("INSERT INTO events (external_match_id, source) VALUES ('', 'sofascore'), ('', 'fotmob'), ('', 'sofascore')").Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	if err := database.PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	var values []string
	if err := db.Raw("SELECT external_match_id FROM events ORDER BY id").Scan(&values).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(values))
	}
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" {
			t.Error("a row was not backfilled")
			continue
		}
		if len(v) < 7 || v[:7] != "legacy-" {
			t.Errorf("row backfilled to %q, want a legacy- prefix", v)
		}
		if seen[v] {
			t.Errorf("backfilled value %q is not unique", v)
		}
		seen[v] = true
	}

	if err := db.Exec("ALTER TABLE events ADD UNIQUE INDEX idx_events_external_match_id (external_match_id)").Error; err != nil {
		t.Fatalf("create the unique index over the backfilled column: %v", err)
	}
}
