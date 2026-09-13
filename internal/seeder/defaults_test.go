package seeder

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper/catalog"
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

// TestSeed_LoadsInitialLeagues verifies that on first boot the FotMob
// catalog is populated with a curated seed of world football leagues
// large enough to give operators a working scraper out of the box.
// The threshold (30) is intentionally below the brief's "~50" target
// so the seed can evolve without breaking the test, but high enough
// to catch accidental truncations of the curated list.
//
// If the catalog admin endpoints have already populated scraper_leagues,
// the seed must be a no-op — operators retain control of which leagues
// are tracked. That is asserted in TestSeed_NonEmptyCatalogIsNoop.
func TestSeed_LoadsInitialLeagues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	if err := SeedDefaults(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var count int64
	if err := db.Model(&catalog.ScraperLeague{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count < 30 {
		t.Fatalf("expected at least 30 leagues seeded, got %d", count)
	}
}

// TestSeed_NonEmptyCatalogIsNoop verifies the seed preserves
// operator-added rows that are not in the curated list. The curated
// seed may run on every boot and reconcile any matching row, but a
// row whose source_league_id is not in the curated slice (e.g. one
// the operator added via /#/scraper-leagues or
// /scraper-leagues/search) must be left untouched — its name,
// enabled flag, and any other field stay as the operator set them.
func TestSeed_NonEmptyCatalogIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	pre := catalog.ScraperLeague{
		Source: "fotmob", SourceLeagueId: "99999",
		Name: "Operator-configured league", Country: "XX", Sport: "football", Enabled: false,
	}
	if err := db.Create(&pre).Error; err != nil {
		t.Fatalf("pre-insert: %v", err)
	}

	if err := SeedDefaults(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The curated seed plus the operator row should both be present.
	var total int64
	if err := db.Unscoped().Model(&catalog.ScraperLeague{}).Count(&total).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != int64(1+len(initialLeagues)) {
		t.Fatalf("expected %d rows (curated + operator), got %d", 1+len(initialLeagues), total)
	}
	// The operator row must survive untouched: same id, same name,
	// same disabled state.
	var got catalog.ScraperLeague
	if err := db.Where("source_league_id = ?", "99999").First(&got).Error; err != nil {
		t.Fatalf("re-read operator row: %v", err)
	}
	if got.Name != "Operator-configured league" {
		t.Errorf("operator row mutated: name = %q", got.Name)
	}
	if got.Enabled {
		t.Errorf("operator row re-enabled: enabled = true")
	}
}

// TestSeed_UpsertsCuratedRowsWhenKeyMatches verifies the reconciliation
// half of SeedDefaults: when a curated row already exists in the DB
// with a stale name/country, SeedDefaults must update those fields to
// the curated values (instead of skipping the entire seed like
// before). The operator's `enabled` flag must be preserved so that
// disabling a curated league survives across boots.
//
// Without the upsert, deployments running an older curated seed
// never get name/ID corrections when the seed evolves — they keep
// the wrong entries and the scraper silently returns no matches for
// them. The unique key for the upsert is (source, source_league_id);
// rows whose key is not in the curated seed must be left alone
// (operators may have added them by hand or via SearchLeagues).
func TestSeed_UpsertsCuratedRowsWhenKeyMatches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// Pre-insert Premier League with deliberately stale values and
	// explicitly enabled=false. SeedDefaults must rewrite name/country/sport
	// to the curated values but leave `enabled=false` alone.
	stale := catalog.ScraperLeague{
		Source: "fotmob", SourceLeagueId: "47",
		Name: "STALE NAME — please overwrite", Country: "ZZ", Sport: "cricket", Enabled: false,
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("pre-insert stale: %v", err)
	}

	if err := SeedDefaults(db, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var got catalog.ScraperLeague
	if err := db.Where("source = ? AND source_league_id = ?", "fotmob", "47").First(&got).Error; err != nil {
		t.Fatalf("re-read curated row: %v", err)
	}
	if got.Name == stale.Name {
		t.Errorf("seed did not overwrite stale name: still %q", got.Name)
	}
	if got.Country == "ZZ" {
		t.Errorf("seed did not overwrite stale country: still %q", got.Country)
	}
	if got.Sport == "cricket" {
		t.Errorf("seed did not overwrite stale sport: still %q", got.Sport)
	}
	if got.Enabled {
		t.Errorf("seed re-enabled a curated row the operator disabled: enabled = true")
	}
}

// TestSeed_OnlySoftDeletedRowsIsNoop covers fix C2 (PR #120 codex
// review) AND the reconciliation behaviour introduced when the
// curated seed evolved.
//
// The catalog DELETE endpoint uses GORM soft-delete (sets DeletedAt).
// GORM's default scoped Count ignores soft-deleted rows and returns
// 0; the pre-PR #120 SeedDefaults used that count to decide whether
// to insert, so the unique index on source_league_id collided with
// the still-present soft-deleted rows and app.New failed at boot.
//
// The PR #120 fix: SeedDefaults must use Unscoped().Count so soft-
// deleted rows count as "table not empty", which is the correct
// semantic for the unique-index guard.
//
// The PR #126 follow-up reconciliation: for a curated row whose
// key IS already in the table (soft-deleted or not), SeedDefaults
// must NOT un-soft-delete it — the operator's "do not scrape this"
// intent must survive a boot. The soft-deleted curated entry is
// skipped; the other 93 curated entries are inserted fresh.
func TestSeed_OnlySoftDeletedRowsIsNoop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&catalog.ScraperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	pre := catalog.ScraperLeague{
		Source: "fotmob", SourceLeagueId: "47",
		Name: "Premier League", Country: "GB", Sport: "football", Enabled: true,
	}
	if err := db.Create(&pre).Error; err != nil {
		t.Fatalf("pre-insert: %v", err)
	}
	// Soft-delete via the catalog DELETE endpoint's pattern:
	// db.Delete(&row). GORM populates DeletedAt and skips the row in
	// default scopes. The underlying row remains and the unique
	// index still applies.
	if err := db.Delete(&pre).Error; err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	// Sanity check: scoped count sees zero rows; unscoped sees one.
	var scoped int64
	if err := db.Model(&catalog.ScraperLeague{}).Count(&scoped).Error; err != nil {
		t.Fatalf("scoped count: %v", err)
	}
	if scoped != 0 {
		t.Fatalf("scoped count expected 0 after soft-delete, got %d", scoped)
	}

	if err := SeedDefaults(db, nil); err != nil {
		t.Fatalf("seed after soft-delete: %v (likely unique-index collision)", err)
	}

	// The soft-deleted curated row for id=47 must still be present
	// (unscoped) and still soft-deleted — the operator's choice is
	// preserved across boots.
	var sd catalog.ScraperLeague
	if err := db.Unscoped().Where("source_league_id = ?", "47").First(&sd).Error; err != nil {
		t.Fatalf("soft-deleted row not found: %v", err)
	}
	if !sd.DeletedAt.Valid {
		t.Error("SeedDefaults un-soft-deleted the curated row; operator intent broken")
	}

	// The other 93 curated entries are inserted fresh.
	var visible int64
	if err := db.Model(&catalog.ScraperLeague{}).Count(&visible).Error; err != nil {
		t.Fatalf("visible count: %v", err)
	}
	if visible != int64(len(initialLeagues)-1) {
		t.Errorf("expected %d visible rows (curated minus the soft-deleted one), got %d",
			len(initialLeagues)-1, visible)
	}
}
