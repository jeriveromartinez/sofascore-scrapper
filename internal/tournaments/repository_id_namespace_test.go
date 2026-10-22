package tournaments

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newNamespaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:tournaments-namespace?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Migrator().DropTable(&Tournament{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := db.AutoMigrate(&Tournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestRepository_Create_AllocatesIDOutsideUpstreamNamespaces is the
// regression test for the tournaments primary-key collision.
//
// Upstream leagues keep the id their source published: FotMob uses the
// raw competition id, 365scores prefixes it with 6_000_000_000. Admin
// created tournaments used to take the table's auto-increment counter,
// which starts at 1 and therefore lands right on top of the FotMob
// competition ids.
func TestRepository_Create_AllocatesIDOutsideUpstreamNamespaces(t *testing.T) {
	db := newNamespaceTestDB(t)
	repo := NewRepository(db)

	upstream := []Tournament{
		{Model: gorm.Model{ID: 1}, Name: "Serie A", Slug: "serie-a"},
		{Model: gorm.Model{ID: 47}, Name: "Premier League", Slug: "premier-league"},
	}
	for i := range upstream {
		if err := db.Create(&upstream[i]).Error; err != nil {
			t.Fatalf("seed upstream league %d: %v", upstream[i].ID, err)
		}
	}

	created, err := repo.Create(context.Background(), "Admin Cup", "admin-cup")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !IsAdminTournamentID(created.ID) {
		t.Fatalf("admin tournament id = %d, want a reserved id in [%d, %d)",
			created.ID, AdminTournamentIDBase, AdminTournamentIDCeiling)
	}
	for _, row := range upstream {
		if row.ID == created.ID {
			t.Fatalf("admin tournament reused the upstream league id %d", row.ID)
		}
	}

	var stored Tournament
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("read back the admin tournament: %v", err)
	}
	if stored.Slug != "admin-cup" {
		t.Fatalf("stored slug = %q, want 'admin-cup'", stored.Slug)
	}
}

// TestRepository_Create_AllocatesDistinctIDs makes sure two admins
// creating a tournament at the same time never end up on the same
// primary key.
func TestRepository_Create_AllocatesDistinctIDs(t *testing.T) {
	db := newNamespaceTestDB(t)
	repo := NewRepository(db)

	first, err := repo.Create(context.Background(), "First", "first")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	second, err := repo.Create(context.Background(), "Second", "second")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("both tournaments got id %d", first.ID)
	}
	if !IsAdminTournamentID(first.ID) || !IsAdminTournamentID(second.ID) {
		t.Fatalf("ids %d and %d are not in the reserved admin band", first.ID, second.ID)
	}
}

// TestRepository_Create_AfterSoftDeletedTournament is the regression
// test for the id allocator.
//
// The allocator looked for the highest live id in the reserved band,
// so a soft-deleted tournament's id was handed out again. The insert
// then collided with the deleted row and every later create failed
// with "could not allocate a tournament id in the reserved admin
// range".
func TestRepository_Create_AfterSoftDeletedTournament(t *testing.T) {
	db := newNamespaceTestDB(t)
	repo := NewRepository(db)

	first, err := repo.Create(context.Background(), "First", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := repo.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}

	second, err := repo.Create(context.Background(), "Second", "second")
	if err != nil {
		t.Fatalf("create after soft delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("the soft-deleted id %d was reused", second.ID)
	}
	if !IsAdminTournamentID(second.ID) {
		t.Fatalf("id %d is outside the reserved admin band", second.ID)
	}

	var stored Tournament
	if err := db.First(&stored, second.ID).Error; err != nil {
		t.Fatalf("read back the second tournament: %v", err)
	}
	if stored.Slug != "second" {
		t.Fatalf("stored slug = %q, want 'second'", stored.Slug)
	}
}
