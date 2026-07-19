package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ChunkStore struct {
	storagePath string
}

func NewChunkStore(storagePath string) *ChunkStore {
	return &ChunkStore{storagePath: storagePath}
}

func (cs *ChunkStore) ChunkDir(uploadID string) string {
	return filepath.Join(cs.storagePath, "chunks", uploadID)
}

func (cs *ChunkStore) ChunkPath(uploadID string, index int) string {
	return filepath.Join(cs.ChunkDir(uploadID), fmt.Sprintf("chunk-%d", index))
}

type ChunkWriteResult struct {
	Hash     string
	ByteSize int64
}

func (cs *ChunkStore) WriteChunk(uploadID string, index int, reader io.Reader) (*ChunkWriteResult, error) {
	dir := cs.ChunkDir(uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create chunk dir: %w", err)
	}

	hash := sha256.New()
	tee := io.TeeReader(io.LimitReader(reader, MaxChunkSize+1), hash)

	tmpPath := filepath.Join(dir, fmt.Sprintf(".chunk-%d-tmp", index))
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open temp chunk: %w", err)
	}

	written, copyErr := io.Copy(f, tee)
	if copyErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write chunk: %w", copyErr)
	}
	if written > MaxChunkSize {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("chunk size exceeds limit of %d bytes", MaxChunkSize)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("fsync chunk: %w", err)
	}
	closeErr := f.Close()
	if closeErr != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("close chunk: %w", closeErr)
	}

	destPath := cs.ChunkPath(uploadID, index)
	if err := atomicRename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("rename chunk: %w", err)
	}

	return &ChunkWriteResult{
		Hash:     hex.EncodeToString(hash.Sum(nil)),
		ByteSize: written,
	}, nil
}

func (cs *ChunkStore) VerifyChunk(uploadID string, index int, expectedHash string) (bool, error) {
	path := cs.ChunkPath(uploadID, index)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return false, err
	}

	return hex.EncodeToString(hash.Sum(nil)) == expectedHash, nil
}

func (cs *ChunkStore) ReadChunk(uploadID string, index int) (*os.File, error) {
	return os.Open(cs.ChunkPath(uploadID, index))
}

func (cs *ChunkStore) RemoveChunks(uploadID string) error {
	dir := cs.ChunkDir(uploadID)
	return os.RemoveAll(dir)
}

func atomicRename(oldpath, newpath string) error {
	if err := os.Rename(oldpath, newpath); err != nil {
		return err
	}

	dir := filepath.Dir(newpath)
	if f, err := os.Open(dir); err == nil {
		f.Sync()
		f.Close()
	}

	return nil
}
