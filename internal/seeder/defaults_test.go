package seeder

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// TestSeed_TruncatesLegacyEvents verifies the first-boot guard: any
// events whose source is empty or "sofascore" must be wiped along
// with their teams/tournaments before the rest of the seed runs.
func TestSeed_TruncatesLegacyEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&events.Event{},
		&events.Team{},
		&tournaments.Tournament{},
		&users.User{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// Seed two rows that the guard must treat as legacy: one explicitly
	// tagged "sofascore" and one with the empty default source that
	// pre-FotMob rows landed at.
	if err := db.Create(&events.Event{ExternalMatchId: "old-1", Source: "sofascore"}).Error; err != nil {
		t.Fatalf("create legacy event 1: %v", err)
	}
	if err := db.Create(&events.Event{ExternalMatchId: "old-2", Source: ""}).Error; err != nil {
		t.Fatalf("create legacy event 2: %v", err)
	}

	if err := SeedDefaults(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var eventCount int64
	if err := db.Model(&events.Event{}).Where("source = ? OR source = ''", "sofascore").Count(&eventCount).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("legacy events no truncados: %d", eventCount)
	}
}
