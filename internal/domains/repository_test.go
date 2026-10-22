package domains

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

func newDomainsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&users.User{}, &Domain{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func seedOwner(t *testing.T, db *gorm.DB) users.User {
	t.Helper()
	owner := users.User{Email: "owner@test.local", Password: "x"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	return owner
}

func seedDomains(t *testing.T, db *gorm.DB, ownerID uint, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := db.Create(&Domain{Domain: name, UserID: ownerID}).Error; err != nil {
			t.Fatalf("seed domain %q: %v", name, err)
		}
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestRepositoryHonoursCancelledContext(t *testing.T) {
	db := newDomainsTestDB(t)
	owner := seedOwner(t, db)
	seedDomains(t, db, owner.ID, "a.example")
	repo := NewRepository(db)
	ctx := cancelledContext()

	checks := map[string]func() error{
		"GetAll":     func() error { _, err := repo.GetAll(ctx); return err },
		"GetByID":    func() error { _, err := repo.GetByID(ctx, 1); return err },
		"ListPage":   func() error { _, _, err := repo.ListPage(ctx, "", 0, 10); return err },
		"ListByUser": func() error { _, err := repo.ListByUser(ctx, owner.ID); return err },
		"Create":     func() error { _, err := repo.Create(ctx, "b.example", owner.ID); return err },
		"Update":     func() error { _, err := repo.Update(ctx, 1, "c.example"); return err },
		"Delete":     func() error { return repo.Delete(ctx, 1) },
	}
	for name, call := range checks {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s with cancelled ctx: want context.Canceled, got %v", name, err)
		}
	}
}

func TestListPageWalksKeysetInDomainOrder(t *testing.T) {
	db := newDomainsTestDB(t)
	owner := seedOwner(t, db)
	seedDomains(t, db, owner.ID, "e.example", "a.example", "d.example", "b.example", "c.example")
	repo := NewRepository(db)

	var walked []string
	cursorDomain, cursorID := "", uint(0)
	for page := 1; ; page++ {
		rows, hasMore, err := repo.ListPage(context.Background(), cursorDomain, cursorID, 2)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, row := range rows {
			if row.User == nil || row.User.ID != owner.ID {
				t.Fatalf("page %d: domain %q missing preloaded owner", page, row.Domain)
			}
			walked = append(walked, row.Domain)
		}
		if !hasMore {
			break
		}
		last := rows[len(rows)-1]
		cursorDomain, cursorID = last.Domain, last.ID
	}

	want := "[a.example b.example c.example d.example e.example]"
	if fmt.Sprint(walked) != want {
		t.Fatalf("walked %v, want %s", walked, want)
	}
}

func TestCreateRejectsUnknownOwner(t *testing.T) {
	repo := NewRepository(newDomainsTestDB(t))

	if _, err := repo.Create(context.Background(), "x.example", 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Create for unknown owner: want gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestDeleteMissingDomainReturnsNotFound(t *testing.T) {
	repo := NewRepository(newDomainsTestDB(t))

	if err := repo.Delete(context.Background(), 42); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Delete missing: want gorm.ErrRecordNotFound, got %v", err)
	}
}
