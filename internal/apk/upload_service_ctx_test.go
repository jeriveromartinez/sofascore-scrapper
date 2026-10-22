package apk

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

func newMiniredisUploadService(t *testing.T) (*UploadService, *dbRecorder) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	db, rec := recordDB(newReconcileTestDB(t))
	return NewUploadService(NewUploadStateStore(client), NewChunkStore(t.TempDir()), NewRepository(db), db), rec
}

func beginTestUpload(t *testing.T, s *UploadService, payload []byte) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	session, err := s.store.Begin(ctx, BeginUploadRequest{
		UserID: 42, FileName: "app.apk", FileSize: int64(len(payload)), TotalChunks: 1, Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("begin upload: %v", err)
	}
	uploadID := uuid.MustParse(session.ID)
	if _, err := s.PutChunk(ctx, uploadID, 0, 42, bytes.NewReader(payload)); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	return uploadID
}

func TestUploadServiceCompleteFailureWritesUseCallerContext(t *testing.T) {
	t.Setenv("APK_STORAGE_PATH", t.TempDir())
	s, rec := newMiniredisUploadService(t)
	uploadID := beginTestUpload(t, s, []byte("not an apk"))

	_, err := s.Complete(markedContext(), uploadID, 42)
	if err == nil || !strings.Contains(err.Error(), "parse APK") {
		t.Fatalf("Complete error = %v, want parse APK failure", err)
	}

	var pub UploadPublication
	if err := s.db.Where("upload_id = ?", uploadID.String()).First(&pub).Error; err != nil {
		t.Fatalf("load publication: %v", err)
	}
	if pub.Status != "failed" {
		t.Fatalf("publication status = %q, want failed", pub.Status)
	}
	inserts := rec.matching("INSERT", "apk_upload_publications")
	updates := rec.matching("UPDATE", "apk_upload_publications")
	if len(inserts) != 1 || len(updates) != 1 {
		t.Fatalf("expected one publication insert and one failure update, got %d/%d", len(inserts), len(updates))
	}
	rec.assertMarked(t, append(inserts, updates...))
}

func TestUploadServiceAbortWritesUseCallerContext(t *testing.T) {
	s, rec := newMiniredisUploadService(t)
	uploadID := beginTestUpload(t, s, []byte("chunk"))
	if err := s.db.Create(&UploadPublication{UploadID: uploadID.String(), Status: "assembling"}).Error; err != nil {
		t.Fatalf("seed publication: %v", err)
	}
	rec.reset()

	if err := s.Abort(markedContext(), uploadID, 42); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	rec.assertMarked(t, append(rec.matching("SELECT", "upload_id"), rec.matching("UPDATE", "")...))

	var pub UploadPublication
	if err := s.db.Where("upload_id = ?", uploadID.String()).First(&pub).Error; err != nil {
		t.Fatalf("load publication: %v", err)
	}
	if pub.Status != "failed" {
		t.Fatalf("publication status = %q, want failed", pub.Status)
	}
}

func TestReconcilePublicationsBulkFailsStaleRowsWithPagedScan(t *testing.T) {
	s, rec := newMiniredisUploadService(t)
	dir := t.TempDir()

	livePath := filepath.Join(dir, "live.apk")
	if err := os.WriteFile(livePath, []byte("apk"), 0o644); err != nil {
		t.Fatalf("write live apk: %v", err)
	}
	rows := []UploadPublication{
		{UploadID: "not-a-uuid", Status: "published"},
		{UploadID: uuid.NewString(), Status: "published", FinalPath: filepath.Join(dir, "missing.apk")},
		{UploadID: uuid.NewString(), Status: "published", FinalPath: livePath},
	}
	var assemblingTemps []string
	for i := 0; i < 3; i++ {
		temp := filepath.Join(dir, uuid.NewString()+".tmp")
		if err := os.WriteFile(temp, []byte("partial"), 0o644); err != nil {
			t.Fatalf("write temp: %v", err)
		}
		assemblingTemps = append(assemblingTemps, temp)
		rows = append(rows, UploadPublication{UploadID: uuid.NewString(), Status: "assembling", TempPath: temp})
	}
	if err := s.db.Create(&rows).Error; err != nil {
		t.Fatalf("seed publications: %v", err)
	}
	ageAssembling(t, s.db, rows[3].ID, rows[4].ID, rows[5].ID)
	rec.reset()

	if err := s.ReconcilePublications(markedContext()); err != nil {
		t.Fatalf("ReconcilePublications: %v", err)
	}

	rec.assertMarked(t, rec.matching("", ""))
	if updates := rec.matching("UPDATE", "apk_upload_publications"); len(updates) != 1 {
		t.Fatalf("expected one bulk UPDATE, got %d: %+v", len(updates), updates)
	}
	for _, sel := range rec.matching("SELECT", "apk_upload_publications") {
		if !strings.Contains(strings.ToUpper(sel.sql), "LIMIT") {
			t.Fatalf("publication scan must be paged: %s", sel.sql)
		}
	}

	var after []UploadPublication
	if err := s.db.Order("id").Find(&after).Error; err != nil {
		t.Fatalf("load publications: %v", err)
	}
	want := []string{"failed", "failed", "published", "failed", "failed", "failed"}
	for i, pub := range after {
		if pub.Status != want[i] {
			t.Errorf("publication %d (%s) status = %q, want %q", pub.ID, pub.UploadID, pub.Status, want[i])
		}
	}
	for _, temp := range assemblingTemps {
		if _, err := os.Stat(temp); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("assembling temp %s not removed: %v", temp, err)
		}
	}
}

func TestReconcilePublicationsHonoursCancelledContext(t *testing.T) {
	s, _ := newMiniredisUploadService(t)
	pub := UploadPublication{UploadID: uuid.NewString(), Status: "assembling"}
	if err := s.db.Create(&pub).Error; err != nil {
		t.Fatalf("seed publication: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.ReconcilePublications(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReconcilePublications error = %v, want context.Canceled", err)
	}

	var after UploadPublication
	if err := s.db.First(&after, pub.ID).Error; err != nil {
		t.Fatalf("load publication: %v", err)
	}
	if after.Status != "assembling" {
		t.Fatalf("status = %q, want untouched assembling", after.Status)
	}
}
