package apk

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestChunkStoreWriteChunkSyncsBeforeClose(t *testing.T) {
	store := NewChunkStore(t.TempDir())
	payload := []byte("chunk data")

	result, err := store.WriteChunk("upload-id", 0, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("WriteChunk returned error: %v", err)
	}
	if result.ByteSize != int64(len(payload)) {
		t.Fatalf("ByteSize = %d, want %d", result.ByteSize, len(payload))
	}

	stored, err := os.ReadFile(store.ChunkPath("upload-id", 0))
	if err != nil {
		t.Fatalf("read stored chunk: %v", err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored chunk = %q, want %q", stored, payload)
	}
}

func TestChunkStoreWriteChunkRejectsOversizedChunk(t *testing.T) {
	store := NewChunkStore(t.TempDir())
	payload := bytes.NewReader(make([]byte, MaxChunkSize+1))

	_, err := store.WriteChunk("upload-id", 0, payload)
	if err == nil || !strings.Contains(err.Error(), "chunk size exceeds limit") {
		t.Fatalf("WriteChunk error = %v, want chunk size limit error", err)
	}
	if _, statErr := os.Stat(store.ChunkPath("upload-id", 0)); !os.IsNotExist(statErr) {
		t.Fatalf("oversized chunk was published: %v", statErr)
	}
}
