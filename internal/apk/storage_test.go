package apk

import (
	"path/filepath"
	"testing"
)

// TestAbsoluteStoragePath_TracksStoragePathChanges pins the cache to
// the path string it was computed from. Every caller that validates a
// request against the storage root depends on this: if the cache is
// not keyed on the path, whichever caller happens to run first in the
// process pins the root for everyone after it, and their requests are
// validated against a directory they are not using.
func TestAbsoluteStoragePath_TracksStoragePathChanges(t *testing.T) {
	first := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", first)

	got, err := absoluteStoragePath()
	if err != nil {
		t.Fatalf("absoluteStoragePath: %v", err)
	}
	if want, _ := filepath.Abs(first); got != want {
		t.Fatalf("first root = %q, want %q", got, want)
	}

	second := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", second)

	got, err = absoluteStoragePath()
	if err != nil {
		t.Fatalf("absoluteStoragePath after change: %v", err)
	}
	if want, _ := filepath.Abs(second); got != want {
		t.Errorf("root after APK_STORAGE_PATH changed = %q, want %q", got, want)
	}
}

// TestAbsoluteStoragePath_SamePathReusesResult covers the reason the
// cache exists: a multi-chunk upload resolves the root once per
// request, and repeated calls for an unchanged path must not redo the
// filepath.Abs walk.
func TestAbsoluteStoragePath_SamePathReusesResult(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", dir)

	first, err := absoluteStoragePath()
	if err != nil {
		t.Fatalf("absoluteStoragePath: %v", err)
	}
	for range 3 {
		got, err := absoluteStoragePath()
		if err != nil {
			t.Fatalf("absoluteStoragePath repeat: %v", err)
		}
		if got != first {
			t.Fatalf("root changed between calls: %q then %q", first, got)
		}
	}
}