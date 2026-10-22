package apk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ageAssembling backdates a publication's updated_at past the
// reconcile grace window, as if its Complete had died long ago.
func ageAssembling(t *testing.T, db *gorm.DB, ids ...uint) {
	t.Helper()
	stale := time.Now().Add(-2 * publicationStaleAfter)
	if err := db.Model(&UploadPublication{}).Where("id IN ?", ids).UpdateColumn("updated_at", stale).Error; err != nil {
		t.Fatalf("age publications: %v", err)
	}
}

func TestReconcilePublicationsSparesInFlightAssembly(t *testing.T) {
	s, _ := newMiniredisUploadService(t)
	dir := t.TempDir()

	freshID := beginTestUpload(t, s, []byte("chunk"))
	freshTemp := filepath.Join(dir, "fresh.tmp")
	staleTemp := filepath.Join(dir, "stale.tmp")
	for _, p := range []string{freshTemp, staleTemp} {
		if err := os.WriteFile(p, []byte("partial"), 0o644); err != nil {
			t.Fatalf("write temp: %v", err)
		}
	}
	fresh := UploadPublication{UploadID: freshID.String(), Status: "assembling", TempPath: freshTemp}
	stale := UploadPublication{UploadID: uuid.NewString(), Status: "assembling", TempPath: staleTemp}
	if err := s.db.Create(&[]*UploadPublication{&fresh, &stale}).Error; err != nil {
		t.Fatalf("seed publications: %v", err)
	}
	ageAssembling(t, s.db, stale.ID)

	if err := s.ReconcilePublications(context.Background()); err != nil {
		t.Fatalf("ReconcilePublications: %v", err)
	}

	var gotFresh, gotStale UploadPublication
	if err := s.db.First(&gotFresh, fresh.ID).Error; err != nil {
		t.Fatalf("load fresh: %v", err)
	}
	if err := s.db.First(&gotStale, stale.ID).Error; err != nil {
		t.Fatalf("load stale: %v", err)
	}
	if gotFresh.Status != "assembling" {
		t.Errorf("in-flight publication status = %q, want assembling", gotFresh.Status)
	}
	if _, err := os.Stat(freshTemp); err != nil {
		t.Errorf("in-flight temp file was removed: %v", err)
	}
	if _, err := s.store.Get(context.Background(), freshID); err != nil {
		t.Errorf("in-flight upload state was aborted: %v", err)
	}
	if gotStale.Status != "failed" {
		t.Errorf("stale publication status = %q, want failed", gotStale.Status)
	}
	if _, err := os.Stat(staleTemp); !os.IsNotExist(err) {
		t.Errorf("stale temp file not removed: %v", err)
	}
}
