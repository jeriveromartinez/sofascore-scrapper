package events

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"gorm.io/gorm"
)

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
	db, err := gorm.Open(sqlite.Open("file:events-tournament-namespace?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
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

	repo := NewRepository(db)
	adminTournament, err := tournaments.NewRepository(db).Create("Admin Cup", "admin-cup")
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
