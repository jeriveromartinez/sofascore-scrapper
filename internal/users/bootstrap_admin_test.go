package users

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestCreateAndMaybeBootstrapAdminPromotesOnlyFirstAccount(t *testing.T) {
	db := newUsersTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	first, promoted, err := repo.CreateAndMaybeBootstrapAdmin(ctx, "first@test.local", strongPassword)
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if !promoted || !first.IsAdmin() {
		t.Fatalf("first registration: promoted=%v role=%q, want promoted admin", promoted, first.Role)
	}

	second, promoted, err := repo.CreateAndMaybeBootstrapAdmin(ctx, "second@test.local", strongPassword)
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if promoted || second.IsAdmin() {
		t.Fatalf("second registration: promoted=%v role=%q, want plain user", promoted, second.Role)
	}

	assertAdminCount(t, db, 1)
}

func TestCreateAndMaybeBootstrapAdminDoesNotPromoteWhenAdminExists(t *testing.T) {
	db := newUsersTestDB(t)
	if err := db.Create(&User{Email: "seeded-admin@test.local", Password: "x", Role: RoleAdmin}).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	user, promoted, err := NewRepository(db).CreateAndMaybeBootstrapAdmin(context.Background(), "late@test.local", strongPassword)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	if promoted || user.IsAdmin() {
		t.Fatalf("registration with existing admin: promoted=%v role=%q, want plain user", promoted, user.Role)
	}
	assertAdminCount(t, db, 1)
}

func TestCreateAndMaybeBootstrapAdminConcurrentRegistrationsYieldOneAdmin(t *testing.T) {
	db := newUsersTestDBShared(t)
	assertConcurrentBootstrapYieldsOneAdmin(t, db, NewRepository(db))
}

func assertConcurrentBootstrapYieldsOneAdmin(t *testing.T, db *gorm.DB, repo *Repository) {
	t.Helper()
	const registrations = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	promotedCount := make(chan bool, registrations)
	errs := make(chan error, registrations)
	for i := range registrations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, promoted, err := repo.CreateAndMaybeBootstrapAdmin(context.Background(), fmt.Sprintf("racer-%d@test.local", i), strongPassword)
			if err != nil {
				errs <- err
				return
			}
			promotedCount <- promoted
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(promotedCount)

	for err := range errs {
		t.Errorf("concurrent registration failed: %v", err)
	}
	promotions := 0
	for promoted := range promotedCount {
		if promoted {
			promotions++
		}
	}
	if promotions != 1 {
		t.Errorf("registrations reporting promotion = %d, want 1", promotions)
	}
	assertAdminCount(t, db, 1)
}

func assertAdminCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var admins int64
	if err := db.Model(&User{}).Where("role = ?", RoleAdmin).Count(&admins).Error; err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if admins != want {
		t.Fatalf("admins = %d, want %d", admins, want)
	}
}
