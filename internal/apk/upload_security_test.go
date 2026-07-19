package apk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ApkVersion{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestValidatePackageName_Valid(t *testing.T) {
	valid := []string{
		"com.example.app",
		"org.company.tool",
		"com.android.vending",
		"co.uk.myapp",
		"Abc.Defg.Hijk",
	}
	for _, pkg := range valid {
		if err := ValidatePackageName(pkg); err != nil {
			t.Errorf("ValidatePackageName(%q) = %v, want nil", pkg, err)
		}
	}
}

func TestValidatePackageName_Invalid(t *testing.T) {
	invalid := []struct {
		name string
		err  error
	}{
		{"com", ErrInvalidPackageName},
		{"com.example", ErrInvalidPackageName},
		{"123.com.example", ErrInvalidPackageName},
		{"com.123.example", ErrInvalidPackageName},
	}
	for _, tc := range invalid {
		err := ValidatePackageName(tc.name)
		if err == nil {
			t.Errorf("ValidatePackageName(%q) = nil, want error", tc.name)
			continue
		}
		if err != tc.err {
			t.Errorf("ValidatePackageName(%q) = %v, want %v", tc.name, err, tc.err)
		}
	}
}

func TestValidatePackageName_PathTraversal(t *testing.T) {
	attacks := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"com\\example\\app",
		"...",
		"..",
		".com.example.app",
		"com.example.app/../../secret",
	}
	for _, pkg := range attacks {
		err := ValidatePackageName(pkg)
		if err == nil {
			t.Errorf("ValidatePackageName(%q) = nil, want path traversal error", pkg)
			continue
		}
		if !errorsAs(err, &ErrPathTraversal) && err.Error() != ErrPathTraversal.Error() {
			t.Errorf("ValidatePackageName(%q) = %v, want ErrPathTraversal", pkg, err)
		}
	}
}

func TestValidatePackageName_SlashAndBackslash(t *testing.T) {
	for _, pkg := range []string{"com/example", "com\\example", "com/../etc"} {
		if err := ValidatePackageName(pkg); err == nil {
			t.Errorf("ValidatePackageName(%q) = nil, want error", pkg)
		}
	}
}

func errorsAs(err error, target *error) bool {
	if err == nil {
		return false
	}
	return err.Error() == (*target).Error()
}

func TestSafeDestination_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	dest, err := SafeDestination(tmpDir, "com.example.app-1.0.0.apk")
	if err != nil {
		t.Fatalf("SafeDestination returned error: %v", err)
	}
	expected := filepath.Join(tmpDir, "com.example.app-1.0.0.apk")
	if dest != expected {
		t.Errorf("SafeDestination = %q, want %q", dest, expected)
	}
}

func TestSafeDestination_Traversal(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := SafeDestination(tmpDir, "../escape.apk")
	if err == nil {
		t.Fatal("SafeDestination with ../ should fail")
	}
}

func TestSafeDestination_DeepTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := SafeDestination(tmpDir, "foo/../../../escape.apk")
	if err == nil {
		t.Fatal("SafeDestination with deep ../ should fail")
	}
}

func TestSafeDestination_NonExistentRoot(t *testing.T) {
	_, err := SafeDestination("/nonexistent/root/path", "file.apk")
	if err != nil {
		t.Logf("expected error on nonexistent root: %v", err)
	}
}

func TestSafeDestination_SymlinkAware(t *testing.T) {
	tmpDir := t.TempDir()
	symlinkPath := filepath.Join(tmpDir, "link")
	realPath := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realPath, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(realPath, symlinkPath)
	dest, err := SafeDestination(symlinkPath, "app.apk")
	if err != nil {
		t.Fatalf("SafeDestination via symlink returned error: %v", err)
	}
	if !strings.HasPrefix(dest, realPath) {
		t.Errorf("SafeDestination = %q, should resolve through symlink to %q", dest, realPath)
	}
}

func TestPublishNoReplace_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source")
	dst := filepath.Join(tmpDir, "destination")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := PublishNoReplace(src, dst); err != nil {
		t.Fatalf("PublishNoReplace failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("PublishNoReplace content = %q, want %q", string(data), "hello")
	}
}

func TestPublishNoReplace_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source")
	dst := filepath.Join(tmpDir, "destination")

	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := PublishNoReplace(src, dst)
	if err == nil {
		t.Fatal("PublishNoReplace should fail when destination exists")
	}

	existing, _ := os.ReadFile(dst)
	if string(existing) != "old" {
		t.Errorf("existing file was modified: got %q, want %q", string(existing), "old")
	}
}

func TestPublishNoReplace_SourceNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	err := PublishNoReplace(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Fatal("PublishNoReplace with nonexistent source should fail")
	}
}

func TestMaxChunkSize(t *testing.T) {
	if MaxChunkSize != 10*1024*1024 {
		t.Errorf("MaxChunkSize = %d, want %d", MaxChunkSize, 10*1024*1024)
	}
}

func TestMaxTotalChunks(t *testing.T) {
	if MaxTotalChunks != 20 {
		t.Errorf("MaxTotalChunks = %d, want %d", MaxTotalChunks, 20)
	}
}

func TestMaxDirectUpload(t *testing.T) {
	if MaxDirectUpload != 200*1024*1024 {
		t.Errorf("MaxDirectUpload = %d, want %d", MaxDirectUpload, 200*1024*1024)
	}
}

func TestMaxAggregate(t *testing.T) {
	if MaxAggregate != 200*1024*1024 {
		t.Errorf("MaxAggregate = %d, want %d", MaxAggregate, 200*1024*1024)
	}
}

func TestUpdateURL_IptvUrlColumn(t *testing.T) {
	db := setupTestDB(t)

	repo := NewRepository(db)
	apk, err := repo.Create("1.0.0", "test.apk", "/tmp/test.apk", "desc", "com.test.app", 1024, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	testURL := "http://test.example.com"
	if err := repo.UpdateURL(apk.ID, testURL); err != nil {
		t.Fatalf("UpdateURL failed: %v", err)
	}

	updated, err := repo.GetByID(apk.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.IPTVUrl != testURL {
		t.Errorf("IPTVUrl = %q, want %q", updated.IPTVUrl, testURL)
	}
}

func TestUpdateURL_NotFound(t *testing.T) {
	db := setupTestDB(t)

	repo := NewRepository(db)
	err := repo.UpdateURL(999999, "http://example.com")
	if err == nil {
		t.Fatal("UpdateURL for nonexistent ID should fail")
	}
}

func TestUpdateURL_RowsAffectedZero(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	err := repo.UpdateURL(0, "http://example.com")
	if err == nil {
		t.Fatal("UpdateURL with id=0 should fail")
	}
}
