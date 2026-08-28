package apk

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newReconcileTestDB opens an in-memory SQLite database with the
// apk_upload_publications table ready for the reconciler.
func newReconcileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UploadPublication{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestReconcilePublications_InvalidUUIDDoesNotPanic verifies that a
// publication row whose upload_id is not a valid UUID does not crash
// the reconciler. With the previous implementation every
// `uuid.MustParse(pub.UploadID)` site panicked, taking the whole
// process down on the next reconciler tick.
func TestReconcilePublications_InvalidUUIDDoesNotPanic(t *testing.T) {
	db := newReconcileTestDB(t)

	pub := UploadPublication{
		UploadID: "not-a-valid-uuid",
		Status:   "published",
	}
	if err := db.Create(&pub).Error; err != nil {
		t.Fatal(err)
	}

	// store and chunkStore are nil; the fix must short-circuit before
	// touching them when the UUID is invalid.
	s := &UploadService{db: db, store: nil, chunkStore: nil}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ReconcilePublications panicked on invalid UUID: %v", r)
		}
	}()

	if err := s.ReconcilePublications(context.Background()); err != nil {
		t.Fatalf("ReconcilePublications returned error: %v", err)
	}

	// The bad row should be marked as failed so the next tick does not
	// see it again.
	var after UploadPublication
	if err := db.First(&after, pub.ID).Error; err != nil {
		t.Fatalf("read back row: %v", err)
	}
	if after.Status != "failed" {
		t.Errorf("status = %q, want %q", after.Status, "failed")
	}
}

// TestReconcilePublications_InvalidUUIDAssemblingAlsoSafe covers the
// `assembling` branch. Same fix applies: parse once, mark failed on
// parse error, continue.
func TestReconcilePublications_InvalidUUIDAssemblingAlsoSafe(t *testing.T) {
	db := newReconcileTestDB(t)

	pub := UploadPublication{
		UploadID: "garbage",
		Status:   "assembling",
	}
	if err := db.Create(&pub).Error; err != nil {
		t.Fatal(err)
	}

	s := &UploadService{db: db, store: nil, chunkStore: nil}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ReconcilePublications panicked on invalid UUID in assembling: %v", r)
		}
	}()

	if err := s.ReconcilePublications(context.Background()); err != nil {
		t.Fatalf("ReconcilePublications returned error: %v", err)
	}

	var after UploadPublication
	if err := db.First(&after, pub.ID).Error; err != nil {
		t.Fatalf("read back row: %v", err)
	}
	if after.Status != "failed" {
		t.Errorf("status = %q, want %q", after.Status, "failed")
	}
}
