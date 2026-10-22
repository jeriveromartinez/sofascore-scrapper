package events

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupReconcileLogosDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Team{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func assertReconcileTeamLogos(t *testing.T, db *gorm.DB, teamIDs [3]int64) {
	t.Helper()
	storage := t.TempDir()
	t.Setenv("IMAGE_STORAGE_PATH", storage)

	remoteID, cachedID, normalizedID := teamIDs[0], teamIDs[1], teamIDs[2]
	seed := []Team{
		{TeamId: remoteID, Name: "Remote", LogoUrl: TeamLogoSourceURL(remoteID)},
		{TeamId: cachedID, Name: "Cached", LogoUrl: TeamLogoSourceURL(cachedID)},
		{TeamId: normalizedID, Name: "Normalized", LogoUrl: TeamLogoAPIPath(normalizedID)},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed teams: %v", err)
	}
	for _, id := range []int64{cachedID, normalizedID} {
		if err := os.MkdirAll(filepath.Dir(TeamLogoLocalPath(id)), 0o755); err != nil {
			t.Fatalf("create logo dir: %v", err)
		}
		if err := os.WriteFile(TeamLogoLocalPath(id), []byte("cached"), 0o644); err != nil {
			t.Fatalf("write cached logo: %v", err)
		}
	}

	recorded, rec := recordSQL(db)
	repo := NewRepository(recorded)
	var scheduled []int64
	repo.scheduleLogo = func(_ *gorm.DB, job LogoJob) {
		scheduled = append(scheduled, job.TeamID)
	}
	if err := repo.ReconcileTeamLogos(context.Background()); err != nil {
		t.Fatalf("ReconcileTeamLogos: %v", err)
	}

	if updates := rec.matching("UPDATE", "teams"); len(updates) != 1 {
		t.Fatalf("expected one bulk UPDATE on teams, got %d: %v", len(updates), updates)
	}
	for _, sel := range rec.matching("SELECT", "teams") {
		if !strings.Contains(strings.ToUpper(sel), "LIMIT") {
			t.Fatalf("team scan must be paged, got unbounded query: %s", sel)
		}
	}

	var teams []Team
	if err := db.Where("team_id IN ?", teamIDs[:]).Order("team_id").Find(&teams).Error; err != nil {
		t.Fatalf("load teams: %v", err)
	}
	for _, team := range teams {
		if team.LogoUrl != TeamLogoAPIPath(team.TeamId) {
			t.Fatalf("team %d logo_url = %q, want %q", team.TeamId, team.LogoUrl, TeamLogoAPIPath(team.TeamId))
		}
	}
	if !reflect.DeepEqual(scheduled, []int64{remoteID}) {
		t.Fatalf("scheduled = %v, want [%d]", scheduled, remoteID)
	}
}

func TestReconcileTeamLogosUsesSingleBulkUpdate_SQLite(t *testing.T) {
	assertReconcileTeamLogos(t, setupReconcileLogosDB(t), [3]int64{10, 20, 30})
}
