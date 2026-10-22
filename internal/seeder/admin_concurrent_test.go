//go:build integration

package seeder_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/seeder"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
)

// TestSeedDefaultAdmin_ConcurrentFirstBoot is the regression test for
// the multi-backend first boot: every backend counted zero users, all
// but one then failed on the duplicate admin@local insert and exited.
// The legacy-truncation branch runs concurrently too.
func TestSeedDefaultAdmin_ConcurrentFirstBoot(t *testing.T) {
	db := newTestDB(t)
	if err := db.Migrator().DropTable(&users.User{}, &events.Event{}, &events.Team{}, &tournaments.Tournament{}); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &events.Event{}, &events.Team{}, &tournaments.Tournament{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	league := tournaments.Tournament{Name: "legacy league"}
	if err := db.Create(&league).Error; err != nil {
		t.Fatalf("create legacy tournament: %v", err)
	}
	for _, team := range []events.Team{{TeamId: 1, Name: "legacy home"}, {TeamId: 2, Name: "legacy away"}} {
		if err := db.Create(&team).Error; err != nil {
			t.Fatalf("create legacy team: %v", err)
		}
	}
	for _, id := range []string{"old-1", "old-2"} {
		e := events.Event{ExternalMatchId: id, Source: "sofascore", HomeTeamId: 1, AwayTeamId: 2, LeagueId: league.ID}
		if err := db.Create(&e).Error; err != nil {
			t.Fatalf("create legacy event: %v", err)
		}
	}

	t.Setenv(seeder.DefaultAdminPasswordEnvVar, "")
	seeder.DefaultAdminPassword = ""
	const backends = 8
	start := make(chan struct{})
	errs := make([]error, backends)
	var wg sync.WaitGroup
	for i := range backends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = seeder.SeedDefaultAdmin(context.Background(), db)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("backend %d: seed: %v", i, err)
		}
	}
	var admins int64
	if err := db.Unscoped().Model(&users.User{}).Where("email = ?", seeder.DefaultAdminEmail).Count(&admins).Error; err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if admins != 1 {
		t.Errorf("admin rows = %d, want 1", admins)
	}
	// Only the call that stored the row may publish its password; a
	// losing call overwriting it would expose a password that does not
	// log in.
	var admin users.User
	if err := db.Where("email = ?", seeder.DefaultAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(seeder.DefaultAdminPassword)); err != nil {
		t.Errorf("DefaultAdminPassword does not match the stored admin: %v", err)
	}
	var legacy int64
	if err := db.Model(&events.Event{}).Count(&legacy).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if legacy != 0 {
		t.Errorf("legacy events left = %d, want 0", legacy)
	}
}
