package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"gorm.io/gorm"
)

// openLegacySQLite opens a sqlite-backed *gorm.DB with the events table
// pre-migrated so the test can simulate the "events table exists from a
// previous deployment" scenario without needing a MySQL fixture. SQLite
// is enough for this test: the helper only emits a single UPDATE and
// we exercise the empty/external_match_id branch.
func openLegacySQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&events.Event{}); err != nil {
		t.Fatalf("automigrate events: %v", err)
	}
	return db
}

// TestPreMigrateLegacyEvents_BackfillsEmptyExternalMatchId simulates the
// scenario codex flagged on PR #118: the events table pre-exists from a
// pre-FotMob deployment, the new external_match_id column gets added by
// the migration, and rows without a value would collide on the new
// unique index. The helper must backfill empty values with
// "legacy-<id>" so the AutoMigrate that creates the unique index
// succeeds.
func TestPreMigrateLegacyEvents_BackfillsEmptyExternalMatchId(t *testing.T) {
	db := openLegacySQLite(t)
	if err := db.Create(&events.Event{ExternalMatchId: "", Source: "sofascore"}).Error; err != nil {
		// SQLite stores empty string for the string column; the row is
		// still allowed because we have not added the unique index yet.
		t.Fatalf("create legacy row: %v", err)
	}

	if err := PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	var row events.Event
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.ExternalMatchId == "" {
		t.Fatalf("ExternalMatchId was not backfilled (still empty)")
	}
	if row.ExternalMatchId[:7] != "legacy-" {
		t.Errorf("ExternalMatchId %q does not start with legacy-", row.ExternalMatchId)
	}
}

// TestPreMigrateLegacyEvents_NoLegacyTableIsNoop verifies the helper is
// safe to call against a fresh DB that has not yet migrated the events
// table. The function must return nil without error.
func TestPreMigrateLegacyEvents_NoLegacyTableIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents on empty DB: %v", err)
	}
}

// TestPreMigrateLegacyEvents_LeavesFreshRowsAlone verifies rows with a
// real external_match_id are NOT touched. The function must only fill
// in missing values, never overwrite populated ones.
func TestPreMigrateLegacyEvents_LeavesFreshRowsAlone(t *testing.T) {
	db := openLegacySQLite(t)
	if err := db.Create(&events.Event{ExternalMatchId: "4193492", Source: "fotmob"}).Error; err != nil {
		t.Fatalf("create fresh row: %v", err)
	}

	if err := PreMigrateLegacyEvents(db); err != nil {
		t.Fatalf("PreMigrateLegacyEvents: %v", err)
	}

	var row events.Event
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.ExternalMatchId != "4193492" {
		t.Errorf("fresh ExternalMatchId was overwritten: %q", row.ExternalMatchId)
	}
}
