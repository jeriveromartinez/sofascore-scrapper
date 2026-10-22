package apk

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// TestChunkStore_StageChunkDoesNotPublish covers the two-phase
// write: a staged chunk is not visible at the published path until it
// is published.
func TestChunkStore_StageChunkDoesNotPublish(t *testing.T) {
	store := NewChunkStore(t.TempDir())

	staged, err := store.StageChunk("upload-id", 0, bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatalf("StageChunk: %v", err)
	}
	if staged.TempPath == "" {
		t.Fatal("staged chunk has no temp path")
	}
	if _, err := os.Stat(store.ChunkPath("upload-id", 0)); !os.IsNotExist(err) {
		t.Fatalf("staged chunk was published: %v", err)
	}

	if err := store.PublishChunk("upload-id", 0, staged); err != nil {
		t.Fatalf("PublishChunk: %v", err)
	}
	stored, err := os.ReadFile(store.ChunkPath("upload-id", 0))
	if err != nil {
		t.Fatalf("read published chunk: %v", err)
	}
	if !bytes.Equal(stored, []byte("payload")) {
		t.Fatalf("published chunk = %q, want %q", stored, "payload")
	}
}

// TestChunkStore_PublishChunkVerifiesHash covers the integrity check
// performed before a chunk is published: a staged file that does not
// match its hash is rejected and the published chunk is left alone.
func TestChunkStore_PublishChunkVerifiesHash(t *testing.T) {
	store := NewChunkStore(t.TempDir())

	published, err := store.WriteChunk("upload-id", 0, bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("WriteChunk: %v", err)
	}

	staged, err := store.StageChunk("upload-id", 0, bytes.NewReader([]byte("tampered")))
	if err != nil {
		t.Fatalf("StageChunk: %v", err)
	}
	if err := os.WriteFile(staged.TempPath, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper staged chunk: %v", err)
	}
	staged.Hash = "0000000000000000000000000000000000000000000000000000000000000000"

	if err := store.PublishChunk("upload-id", 0, staged); err == nil {
		t.Fatal("PublishChunk accepted a chunk whose hash does not match")
	}

	stored, err := os.ReadFile(store.ChunkPath("upload-id", 0))
	if err != nil {
		t.Fatalf("read published chunk: %v", err)
	}
	if !bytes.Equal(stored, []byte("original")) {
		t.Fatalf("published chunk = %q, want %q", stored, "original")
	}
	_ = published
}

// TestChunkStore_DiscardChunkRemovesStagedFile covers the cleanup of
// a rejected chunk.
func TestChunkStore_DiscardChunkRemovesStagedFile(t *testing.T) {
	store := NewChunkStore(t.TempDir())

	staged, err := store.StageChunk("upload-id", 0, bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatalf("StageChunk: %v", err)
	}
	store.DiscardChunk(staged)

	if _, err := os.Stat(staged.TempPath); !os.IsNotExist(err) {
		t.Fatalf("staged chunk was not removed: %v", err)
	}
}

// TestChunkStore_ConcurrentWritesSameChunkAreAtomic covers the race
// between two writers of the same chunk index: the published chunk
// must hold exactly one of the written payloads, never a mix.
func TestChunkStore_ConcurrentWritesSameChunkAreAtomic(t *testing.T) {
	store := NewChunkStore(t.TempDir())

	payloads := [][]byte{
		bytes.Repeat([]byte("a"), 64*1024),
		bytes.Repeat([]byte("b"), 64*1024),
		bytes.Repeat([]byte("c"), 64*1024),
		bytes.Repeat([]byte("d"), 64*1024),
	}

	var wg sync.WaitGroup
	for _, payload := range payloads {
		wg.Add(1)
		go func(p []byte) {
			defer wg.Done()
			if _, err := store.WriteChunk("upload-id", 0, bytes.NewReader(p)); err != nil {
				// Windows refuses to replace a file that another
				// writer is renaming onto at the same time. The
				// property under test is that the published chunk
				// is never a mix, so the failed rename is only
				// tolerated on that platform.
				if runtime.GOOS == "windows" && strings.Contains(err.Error(), "Access is denied") {
					return
				}
				t.Errorf("WriteChunk: %v", err)
			}
		}(payload)
	}
	wg.Wait()

	stored, err := os.ReadFile(store.ChunkPath("upload-id", 0))
	if err != nil {
		t.Fatalf("read published chunk: %v", err)
	}
	matched := false
	for _, payload := range payloads {
		if bytes.Equal(stored, payload) {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("published chunk holds a mix of the concurrent writes (%d bytes)", len(stored))
	}

	leftovers := stagedChunkFiles(t, store.ChunkDir("upload-id"))
	if len(leftovers) != 0 {
		t.Fatalf("staged chunk files left behind: %v", leftovers)
	}
}

// TestChunkStore_CleanupStaleFilesRemovesLeftovers covers the reaper
// for interrupted uploads: stale staged files and legacy chunk
// directories are removed, fresh ones are kept.
func TestChunkStore_CleanupStaleFilesRemovesLeftovers(t *testing.T) {
	root := t.TempDir()
	store := NewChunkStore(root)

	staleDir := store.ChunkDir("stale-upload")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("create stale dir: %v", err)
	}
	staleFile := filepath.Join(staleDir, ".chunk-0-123.tmp")
	if err := os.WriteFile(staleFile, []byte("partial"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	old := time.Now().Add(-2 * ChunkTempMaxAge)
	if err := os.Chtimes(staleFile, old, old); err != nil {
		t.Fatalf("age stale file: %v", err)
	}
	oldDir := time.Now().Add(-2 * ChunkDirMaxAge)
	if err := os.Chtimes(staleDir, oldDir, oldDir); err != nil {
		t.Fatalf("age stale dir: %v", err)
	}

	freshDir := store.ChunkDir("fresh-upload")
	if err := os.MkdirAll(freshDir, 0o755); err != nil {
		t.Fatalf("create fresh dir: %v", err)
	}
	freshFile := filepath.Join(freshDir, ".chunk-0-456.tmp")
	if err := os.WriteFile(freshFile, []byte("partial"), 0o644); err != nil {
		t.Fatalf("write fresh file: %v", err)
	}

	removed, err := store.CleanupStaleFiles(time.Now(), ChunkTempMaxAge, ChunkDirMaxAge)
	if err != nil {
		t.Fatalf("CleanupStaleFiles: %v", err)
	}
	if removed == 0 {
		t.Fatal("nothing was removed")
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("stale chunk dir survived: %v", err)
	}
	if _, err := os.Stat(freshFile); err != nil {
		t.Fatalf("fresh staged file was removed: %v", err)
	}
}

// TestCleanupJob_RunRemovesStaleLeftovers covers the wiring of the
// reaper into the scheduled cleanup job.
func TestCleanupJob_RunRemovesStaleLeftovers(t *testing.T) {
	root := t.TempDir()
	store := NewChunkStore(root)

	legacyDir := store.ChunkDir("legacy-upload")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	old := time.Now().Add(-2 * ChunkDirMaxAge)
	if err := os.Chtimes(legacyDir, old, old); err != nil {
		t.Fatalf("age legacy dir: %v", err)
	}

	job := NewCleanupJob(nil, store)
	if err := job.Run(context.Background(), nil); err != nil {
		t.Fatalf("cleanup job: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy chunk dir survived the cleanup job: %v", err)
	}
}

// TestUploadService_PutChunk_RejectedChunkKeepsStoredChunk covers
// the upload path: a chunk the upload state rejects must not replace
// the chunk that was already accepted.
func TestUploadService_PutChunk_RejectedChunkKeepsStoredChunk(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewUploadStateStore(client)
	chunkStore := NewChunkStore(t.TempDir())
	service := NewUploadService(store, chunkStore, nil, nil)

	ctx := context.Background()
	session, err := store.Begin(ctx, BeginUploadRequest{
		UserID: 42, FileName: "app.apk", FileSize: 8, TotalChunks: 1,
	})
	if err != nil {
		t.Fatalf("begin upload: %v", err)
	}
	uploadID, err := uuid.Parse(session.ID)
	if err != nil {
		t.Fatalf("parse upload id: %v", err)
	}

	if _, err := service.PutChunk(ctx, uploadID, 0, 42, bytes.NewReader([]byte("first"))); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	_, err = service.PutChunk(ctx, uploadID, 0, 42, bytes.NewReader([]byte("other")))
	if err == nil {
		t.Fatal("second PutChunk with a different payload was accepted")
	}
	if !strings.Contains(err.Error(), "chunk hash mismatch") {
		t.Fatalf("error = %v, want chunk hash mismatch", err)
	}

	stored, err := os.ReadFile(chunkStore.ChunkPath(session.ID, 0))
	if err != nil {
		t.Fatalf("read stored chunk: %v", err)
	}
	if !bytes.Equal(stored, []byte("first")) {
		t.Fatalf("stored chunk = %q, want %q", stored, "first")
	}
}

func stagedChunkFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read chunk dir: %v", err)
	}
	var staged []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".chunk-") {
			staged = append(staged, entry.Name())
		}
	}
	return staged
}
