package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ChunkTempMaxAge is how long a staged chunk file may live before
	// it is treated as the leftover of an interrupted upload.
	ChunkTempMaxAge = time.Hour

	// ChunkDirMaxAge is how long a chunk directory may live before it
	// is treated as a legacy leftover. Upload sessions expire after
	// 24h, so anything older than that was never cleaned up.
	ChunkDirMaxAge = 48 * time.Hour
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

	// TempPath is the staged file holding the chunk. It is published
	// by PublishChunk and removed by DiscardChunk.
	TempPath string
}

// StageChunk writes the chunk to a temporary file inside the upload's
// chunk directory and returns its hash. The chunk is not published
// yet: a chunk the upload state rejects must never replace the
// stored one.
//
// The temporary file name is unique per writer, so two concurrent
// uploads of the same chunk index cannot write to the same file.
func (cs *ChunkStore) StageChunk(uploadID string, index int, reader io.Reader) (*ChunkWriteResult, error) {
	dir := cs.ChunkDir(uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create chunk dir: %w", err)
	}

	hash := sha256.New()
	tee := io.TeeReader(io.LimitReader(reader, MaxChunkSize+1), hash)

	f, err := os.CreateTemp(dir, fmt.Sprintf(".chunk-%d-*.tmp", index))
	if err != nil {
		return nil, fmt.Errorf("create temp chunk: %w", err)
	}
	tmpPath := f.Name()

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
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("chmod temp chunk: %w", err)
	}

	return &ChunkWriteResult{
		Hash:     hex.EncodeToString(hash.Sum(nil)),
		ByteSize: written,
		TempPath: tmpPath,
	}, nil
}

// PublishChunk verifies the staged chunk against its hash and renames
// it into place. The rename is atomic, so a reader never observes a
// half-written chunk.
func (cs *ChunkStore) PublishChunk(uploadID string, index int, staged *ChunkWriteResult) error {
	if staged == nil || staged.TempPath == "" {
		return fmt.Errorf("no staged chunk to publish")
	}
	actual, err := fileHash(staged.TempPath)
	if err != nil {
		os.Remove(staged.TempPath)
		return fmt.Errorf("hash staged chunk: %w", err)
	}
	if actual != staged.Hash {
		os.Remove(staged.TempPath)
		return fmt.Errorf("staged chunk hash mismatch")
	}
	if err := atomicRename(staged.TempPath, cs.ChunkPath(uploadID, index)); err != nil {
		os.Remove(staged.TempPath)
		return fmt.Errorf("rename chunk: %w", err)
	}
	return nil
}

// DiscardChunk removes a staged chunk that was not published.
func (cs *ChunkStore) DiscardChunk(staged *ChunkWriteResult) {
	if staged == nil || staged.TempPath == "" {
		return
	}
	os.Remove(staged.TempPath)
}

// WriteChunk stages the chunk and publishes it in one step.
func (cs *ChunkStore) WriteChunk(uploadID string, index int, reader io.Reader) (*ChunkWriteResult, error) {
	staged, err := cs.StageChunk(uploadID, index, reader)
	if err != nil {
		return nil, err
	}
	if err := cs.PublishChunk(uploadID, index, staged); err != nil {
		return nil, err
	}
	return staged, nil
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

// CleanupStaleFiles removes the leftovers of interrupted uploads:
// staged chunk files that were never published and chunk directories
// whose upload session is long gone. It returns the number of removed
// entries.
func (cs *ChunkStore) CleanupStaleFiles(now time.Time, tempMaxAge, dirMaxAge time.Duration) (int, error) {
	root := filepath.Join(cs.storagePath, "chunks")
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, dirEntry := range dirs {
		if !dirEntry.IsDir() {
			continue
		}
		dir := filepath.Join(root, dirEntry.Name())

		// The age of the directory is read before the leftovers are
		// removed: unlinking a file bumps the parent directory mtime,
		// which would make an abandoned directory look fresh again.
		dirInfo, err := os.Stat(dir)
		if err != nil {
			continue
		}
		dirAge := now.Sub(dirInfo.ModTime())

		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), ".chunk-") {
				continue
			}
			info, err := file.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) < tempMaxAge {
				continue
			}
			if err := os.Remove(filepath.Join(dir, file.Name())); err == nil {
				removed++
			}
		}

		if dirAge < dirMaxAge {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			removed++
		}
	}

	return removed, nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
