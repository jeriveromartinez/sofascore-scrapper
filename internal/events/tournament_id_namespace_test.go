package events

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

// eventsNamespaceDBCounter hands out the name of the in-memory database
// each call gets. A shared-cache database survives until its last connection
// closes, so the name has to be unique per call.
var eventsNamespaceDBCounter atomic.Int64

// openNamespaceTestDB opens an in-memory database holding the tables the
// tournament id namespace touches.
func openNamespaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// The name is unique per call so a database left behind by an earlier
	// run cannot be picked up again, and the pool is closed so the database
	// does not outlive the test.
	dsn := fmt.Sprintf("file:events-tournament-namespace-%d?mode=memory&cache=shared", eventsNamespaceDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrator().DropTable(&Event{}); err != nil {
		t.Fatalf("drop events table: %v", err)
	}
	if err := db.Migrator().DropTable(&Team{}); err != nil {
		t.Fatalf("drop teams table: %v", err)
	}
	if err := db.Migrator().DropTable(&tournaments.Tournament{}); err != nil {
		t.Fatalf("drop tournaments table: %v", err)
	}
	if err := db.AutoMigrate(&Team{}, &tournaments.Tournament{}, &Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestOpenNamespaceTestDBUsesAFreshDatabasePerCall pins the hermeticity of
// the in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one wrote to.
func TestOpenNamespaceTestDBUsesAFreshDatabasePerCall(t *testing.T) {
	first := openNamespaceTestDB(t)
	if err := first.Exec("CREATE TABLE marker (id INTEGER)").Error; err != nil {
		t.Fatalf("create marker table: %v", err)
	}

	second := openNamespaceTestDB(t)
	if second.Migrator().HasTable("marker") {
		t.Fatal("the second call reused the database of the first one")
	}
}

// TestUpsertScrapeBatch_DoesNotClobberAdminTournament is the
// end-to-end regression test for the tournaments primary-key
// collision.
//
// The scraper upserts upstream leagues into the tournaments table by
// primary key. An admin-created tournament that drew its id from the
// table's auto-increment counter landed on a FotMob competition id, so
// the next scrape tick rewrote the admin's row with the upstream
// league's name and slug.
func TestUpsertScrapeBatch_DoesNotClobberAdminTournament(t *testing.T) {
	db := openNamespaceTestDB(t)

	repo := NewRepository(db)
	adminTournament, err := tournaments.NewRepository(db).Create(context.Background(), "Admin Cup", "admin-cup")
	if err != nil {
		t.Fatalf("create admin tournament: %v", err)
	}

	// The upstream league the scraper publishes under the raw FotMob
	// competition id 1.
	batch := ScrapeBatch{
		Tournaments: []tournaments.Tournament{
			{Model: gorm.Model{ID: 1}, Name: "Serie A", Slug: "serie-a"},
		},
	}
	if err := repo.UpsertScrapeBatch(context.Background(), batch, 500); err != nil {
		t.Fatalf("UpsertScrapeBatch failed: %v", err)
	}

	var admin tournaments.Tournament
	if err := db.First(&admin, adminTournament.ID).Error; err != nil {
		t.Fatalf("the admin tournament disappeared: %v", err)
	}
	if admin.Name != "Admin Cup" || admin.Slug != "admin-cup" {
		t.Fatalf("the upstream league overwrote the admin tournament: name=%q slug=%q", admin.Name, admin.Slug)
	}

	var upstream tournaments.Tournament
	if err := db.First(&upstream, 1).Error; err != nil {
		t.Fatalf("the upstream league was not stored: %v", err)
	}
	if upstream.Slug != "serie-a" {
		t.Fatalf("upstream slug = %q, want 'serie-a'", upstream.Slug)
	}
}
