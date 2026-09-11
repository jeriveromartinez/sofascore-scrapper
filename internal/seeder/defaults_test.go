package seeder

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// TestSeedDefaultAdmin_TruncatesLegacyEvents verifies the first-boot
// guard folded into SeedDefaultAdmin: any events whose source is empty
// or "sofascore" must be wiped along with their teams/tournaments
// before the default admin is seeded.
func TestSeedDefaultAdmin_TruncatesLegacyEvents(t *testing.T) {
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
	// Seed two rows the guard must treat as legacy: one explicitly
	// tagged "sofascore" and one with the empty default source that
	// pre-FotMob rows landed at. Plus a team that the guard must also
	// wipe, since the truncation branch is "all or nothing".
	if err := db.Create(&events.Event{ExternalMatchId: "old-1", Source: "sofascore"}).Error; err != nil {
		t.Fatalf("create legacy event 1: %v", err)
	}
	if err := db.Create(&events.Event{ExternalMatchId: "old-2", Source: ""}).Error; err != nil {
		t.Fatalf("create legacy event 2: %v", err)
	}
	if err := db.Create(&events.Team{TeamId: 1, Name: "legacy team"}).Error; err != nil {
		t.Fatalf("create legacy team: %v", err)
	}

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Guard outcome: legacy events are gone.
	var eventCount int64
	if err := db.Model(&events.Event{}).Where("source = ? OR source = ''", "sofascore").Count(&eventCount).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("legacy events not truncated: %d", eventCount)
	}
	// Guard outcome: associated team was wiped too.
	var teamCount int64
	if err := db.Model(&events.Team{}).Count(&teamCount).Error; err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if teamCount != 0 {
		t.Errorf("legacy team not truncated: %d rows remain", teamCount)
	}
	// Side effect: default admin is created in the same call.
	var admin users.User
	if err := db.Where("email = ?", DefaultAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("default admin not created: %v", err)
	}
	if admin.Role != users.RoleAdmin {
		t.Errorf("admin role = %q, want %q", admin.Role, users.RoleAdmin)
	}
}

// TestSeedDefaultAdmin_NoLegacyIsNoop verifies the guard does not
// touch fresh FotMob data and that the default admin is still seeded.
func TestSeedDefaultAdmin_NoLegacyIsNoop(t *testing.T) {
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
	// One fresh FotMob row, NOT a legacy row. The guard must leave
	// it alone.
	if err := db.Create(&events.Event{ExternalMatchId: "new-1", Source: "fotmob"}).Error; err != nil {
		t.Fatalf("create new event: %v", err)
	}

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Fresh event survives.
	var eventCount int64
	if err := db.Model(&events.Event{}).Count(&eventCount).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("fresh event was wiped: count = %d, want 1", eventCount)
	}
	// Admin is still seeded.
	var admin users.User
	if err := db.Where("email = ?", DefaultAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("default admin not created: %v", err)
	}
}

// TestSeedDefaultAdmin_EmptyEventsTableIsNoop verifies the guard is
// safe to call against a DB that has the users table but not the
// events/teams/tournaments tables yet (e.g. an existing integration
// test that only migrates users). The function must not error out on
// the missing-tables path; it should just skip the guard and proceed
// to admin creation.
func TestSeedDefaultAdmin_EmptyEventsTableIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var admin users.User
	if err := db.Where("email = ?", DefaultAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("default admin not created: %v", err)
	}
}
