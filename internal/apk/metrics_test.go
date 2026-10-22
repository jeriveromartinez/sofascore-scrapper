package apk

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
)

// probeReader records the active-upload gauge while the chunk body is
// being consumed, i.e. while the upload is in flight.
type probeReader struct {
	t        *testing.T
	r        io.Reader
	observed float64
}

func (p *probeReader) Read(b []byte) (int, error) {
	p.observed = metricValue(p.t, observability.UploadActive)
	return p.r.Read(b)
}

func TestUploadServiceMetrics(t *testing.T) {
	t.Setenv("APK_STORAGE_PATH", t.TempDir())
	s, _ := newMiniredisUploadService(t)
	ctx := context.Background()

	payload := "not an apk payload"
	session, err := s.store.Begin(ctx, BeginUploadRequest{UserID: 42, FileName: "app.apk", FileSize: int64(len(payload)), TotalChunks: 1, Version: "1.0.0"})
	if err != nil {
		t.Fatalf("begin upload: %v", err)
	}
	uploadID := uuid.MustParse(session.ID)

	active := metricValue(t, observability.UploadActive)
	bytesBefore := metricValue(t, observability.UploadBytes)
	failuresBefore := metricValue(t, observability.UploadFailures)

	probe := &probeReader{t: t, r: strings.NewReader(payload)}
	if _, err := s.PutChunk(ctx, uploadID, 0, 42, probe); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	if probe.observed != active+1 {
		t.Fatalf("upload_active during PutChunk = %v, want %v", probe.observed, active+1)
	}
	if got := metricValue(t, observability.UploadBytes) - bytesBefore; got != float64(len(payload)) {
		t.Fatalf("upload_bytes_total delta = %v, want %d", got, len(payload))
	}

	if _, err := s.Complete(ctx, uploadID, 42); err == nil {
		t.Fatal("Complete accepted a payload that is not an APK")
	}
	if got := metricValue(t, observability.UploadFailures) - failuresBefore; got != 1 {
		t.Fatalf("upload_failures_total delta = %v, want 1", got)
	}
	if got := metricValue(t, observability.UploadActive); got != active {
		t.Fatalf("upload_active after uploads finished = %v, want %v", got, active)
	}
}

func TestDownloadCounterReportsBufferedDownloads(t *testing.T) {
	counter, _ := newMiniredisDownloadCounter(t)
	apks := []ApkVersion{
		{Version: "1.0.0", FileName: "a.apk", FilePath: "/tmp/a.apk", PackageName: "com.test.buf.a", DownloadToken: "buf-a"},
		{Version: "1.0.0", FileName: "b.apk", FilePath: "/tmp/b.apk", PackageName: "com.test.buf.b", DownloadToken: "buf-b"},
	}
	if err := counter.db.Create(&apks).Error; err != nil {
		t.Fatalf("seed apks: %v", err)
	}
	ctx := context.Background()
	for _, id := range []uint{apks[0].ID, apks[0].ID, apks[1].ID} {
		if err := counter.Increment(ctx, id); err != nil {
			t.Fatalf("Increment: %v", err)
		}
	}
	if got := metricValue(t, observability.APKDownloadsBuffered); got != 3 {
		t.Fatalf("apk_downloads_buffered = %v, want 3", got)
	}

	if err := counter.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := metricValue(t, observability.APKDownloadsBuffered); got != 0 {
		t.Fatalf("apk_downloads_buffered after flush = %v, want 0", got)
	}
}

func TestReconcilePublicationsCountsFailures(t *testing.T) {
	s, _ := newMiniredisUploadService(t)
	rows := []UploadPublication{
		{UploadID: uuid.NewString(), Status: "assembling"},
		{UploadID: "not-a-uuid", Status: "published"},
	}
	if err := s.db.Create(&rows).Error; err != nil {
		t.Fatalf("seed publications: %v", err)
	}
	before := metricValue(t, observability.UploadFailures)

	if err := s.ReconcilePublications(context.Background()); err != nil {
		t.Fatalf("ReconcilePublications: %v", err)
	}
	if got := metricValue(t, observability.UploadFailures) - before; got != 2 {
		t.Fatalf("upload_failures_total delta = %v, want 2", got)
	}
}
