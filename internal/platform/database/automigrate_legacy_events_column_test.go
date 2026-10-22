package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"gorm.io/gorm"
)

// openLegacyEventsTableWithoutColumn creates an events table shaped
// like the pre-FotMob-reset schema: the table exists but the
// external_match_id column does not. Deployments upgrading from that
// schema used to abort the boot with "Unknown column
// 'external_match_id' in 'where clause'".
func openLegacyEventsTableWithoutColumn(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL DEFAULT '',
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create legacy events table: %v", err)
	}
	return db
}

// TestPreMigrateLegacyEvents_AddsMissingColumn covers the upgrade path
// from a schema that predates the external_match_id column.
func TestPreMigrateLegacyEvents_AddsMissingColumn(t *testing.T) {
	db := openLegacyEventsTableWithoutColumn(t)
	if err := db.Exec("INSERT INTO events (id, source) VALUES (1, 'sofascore'), (2, 'fotmob')").Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	if err := PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	if !db.Migrator().HasColumn(&events.Event{}, "external_match_id") {
		t.Fatal("the external_match_id column was not added")
	}

	var rows []struct {
		ID              int64
		ExternalMatchID string
	}
	if err := db.Raw("SELECT id, external_match_id FROM events ORDER BY id").Scan(&rows).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 backfilled rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ExternalMatchID == "" {
			t.Errorf("row %d was not backfilled", r.ID)
		}
		if r.ExternalMatchID[:7] != "legacy-" {
			t.Errorf("row %d backfilled to %q, want a legacy- prefix", r.ID, r.ExternalMatchID)
		}
	}
	if rows[0].ExternalMatchID == rows[1].ExternalMatchID {
		t.Errorf("backfilled values collide: %q", rows[0].ExternalMatchID)
	}
}

// TestPreMigrateLegacyEvents_BackfillsSoftDeletedRows asserts rows
// hidden by the soft-delete filter are backfilled too: the unique
// index spans soft-deleted rows, so leaving them empty breaks the
// migration just the same.
func TestPreMigrateLegacyEvents_BackfillsSoftDeletedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&events.Event{}); err != nil {
		t.Fatalf("automigrate events: %v", err)
	}
	row := events.Event{ExternalMatchId: "", Source: "sofascore"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create row: %v", err)
	}
	if err := db.Delete(&row).Error; err != nil {
		t.Fatalf("soft delete row: %v", err)
	}

	if err := PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	var value string
	if err := db.Raw("SELECT external_match_id FROM events WHERE id = ?", row.ID).Scan(&value).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value == "" {
		t.Fatal("the soft-deleted row was not backfilled")
	}
}

// TestLegacyExternalMatchIDExpr pins the dialect split: MySQL and
// MariaDB parse "||" as logical OR unless PIPES_AS_CONCAT is in
// sql_mode, which gave every legacy row the same value and broke the
// unique index.
func TestLegacyExternalMatchIDExpr(t *testing.T) {
	if got := legacyExternalMatchIDExpr("mysql"); got != "CONCAT('legacy-', id)" {
		t.Errorf("mysql expression = %q, want CONCAT()", got)
	}
	if got := legacyExternalMatchIDExpr("sqlite"); got != "'legacy-' || id" {
		t.Errorf("sqlite expression = %q, want the || operator", got)
	}
}
