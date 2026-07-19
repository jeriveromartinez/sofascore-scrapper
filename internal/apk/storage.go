package apk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
