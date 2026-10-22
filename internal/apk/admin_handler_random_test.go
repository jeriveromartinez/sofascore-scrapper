package apk

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// failRandomSource makes the random source fail for the duration of
// the test.
func failRandomSource(t *testing.T) {
	t.Helper()
	previous := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("entropy source failed") }
	t.Cleanup(func() { randRead = previous })
}

// TestHandleUploadFailsWhenRandomSourceFails covers the temporary file
// name: a failing random source must abort the upload instead of
// writing the file under a name derived from zeroed bytes.
func TestHandleUploadFailsWhenRandomSourceFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storagePath := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", storagePath)
	failRandomSource(t)

	router := gin.New()
	router.POST("/apk/upload", NewAdminHandler(nil).handleUpload)

	rec := postMultipart(t, router, "/apk/upload", map[string]string{}, "file", []byte("apk bytes"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%q)", rec.Code, rec.Body.String())
	}

	entries, err := os.ReadDir(storagePath)
	if err != nil {
		t.Fatalf("read storage dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("the upload wrote %d files despite the random failure", len(entries))
	}
}

// TestHandleAssembleChunksFailsWhenRandomSourceFails covers the same
// failure on the legacy chunk assembly path.
func TestHandleAssembleChunksFailsWhenRandomSourceFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storagePath := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", storagePath)
	failRandomSource(t)

	const userID = uint(42)
	uploadID := uuid.NewString()
	chunkDir := filepath.Join(storagePath, "legacy-chunks", "42", uploadID)
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		t.Fatalf("create chunk dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chunkDir, "chunk-0"), []byte("chunk"), 0o644); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	router := gin.New()
	router.POST("/apk/upload/assemble", func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	}, NewAdminHandler(nil).handleAssembleChunks)

	rec := postMultipart(t, router, "/apk/upload/assemble", map[string]string{
		"upload_id":    uploadID,
		"total_chunks": "1",
	}, "", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%q)", rec.Code, rec.Body.String())
	}

	entries, err := os.ReadDir(storagePath)
	if err != nil {
		t.Fatalf("read storage dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "legacy-chunks" {
		t.Fatalf("the assembly wrote files despite the random failure: %v", entries)
	}
}

// TestRandomSuffixReportsRandomFailure covers the helper itself: the
// error must reach the caller instead of yielding a zero suffix.
func TestRandomSuffixReportsRandomFailure(t *testing.T) {
	failRandomSource(t)

	if _, err := randomSuffix(); err == nil {
		t.Fatal("expected the random failure to be reported")
	}
}

// postMultipart sends a multipart request with the given form fields
// and an optional file part.
func postMultipart(t *testing.T, router http.Handler, path string, fields map[string]string, fileField string, fileContent []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if fileField != "" {
		part, err := writer.CreateFormFile(fileField, "upload.apk")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(fileContent); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
