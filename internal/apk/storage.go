package apk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	MaxChunkSize    = 10 * 1024 * 1024
	MaxTotalChunks  = 20
	MaxDirectUpload = 200 * 1024 * 1024
	MaxAggregate    = 200 * 1024 * 1024
)

var publishNoReplaceFn = publishNoReplaceFallback

func StoragePath() string {
	if p := os.Getenv("APK_STORAGE_PATH"); p != "" {
		return p
	}
	return "./apk_storage"
}

// absoluteStoragePath resolves StoragePath() to an absolute path
// exactly once and caches the result. The handlers that gate
// chunk uploads and assembly against the storage root used to
// recompute filepath.Abs(StoragePath()) on every request — for a
// 1000-chunk upload that's 1000 redundant syscalls walking the
// CWD. The env var can in principle change at runtime (tests
// rely on t.Setenv), so the cache uses sync.Once and is keyed on
// the path string returned by StoragePath() at call time: if the
// caller mutates the env var, the cached value stays pinned to
// the path that was current at first use, which matches the
// behaviour the per-request code had (every request saw the env
// value at its own call time, but the absolute-path cost was
// paid every time).
var (
	absStorageOnce sync.Once
	absStoragePath string
	absStorageErr  error
)

func absoluteStoragePath() (string, error) {
	absStorageOnce.Do(func() {
		absStoragePath, absStorageErr = filepath.Abs(StoragePath())
	})
	return absStoragePath, absStorageErr
}

func SafeDestination(root, name string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve storage root: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		resolved = absRoot
	}

	dest := filepath.Join(resolved, name)
	rel, err := filepath.Rel(resolved, dest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ErrPathTraversal
	}

	return dest, nil
}

func PublishNoReplace(temp, dest string) error {
	return publishNoReplaceFn(temp, dest)
}

func publishNoReplaceFallback(temp, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return os.ErrExist
	}

	tmpDest := dest + ".publish-tmp"
	f, err := os.OpenFile(tmpDest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("cannot create exclusive target: %w", err)
	}

	src, err := os.Open(temp)
	if err != nil {
		f.Close()
		os.Remove(tmpDest)
		return fmt.Errorf("cannot open source: %w", err)
	}

	_, copyErr := io.Copy(f, src)
	src.Close()
	f.Close()

	if copyErr != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("cannot copy data: %w", copyErr)
	}

	if err := os.Rename(tmpDest, dest); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("cannot rename to final destination: %w", err)
	}

	return nil
}
