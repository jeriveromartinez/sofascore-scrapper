package apk

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestLegacyChunkUploadsAreIsolatedByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	storagePath := t.TempDir()
	t.Setenv("APK_STORAGE_PATH", storagePath)
	uploadID := uuid.NewString()

	router := gin.New()
	router.POST("/apk/upload/chunk", func(c *gin.Context) {
		userID, ok := c.GetQuery("user")
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		var parsed uint
		if _, err := fmt.Sscanf(userID, "%d", &parsed); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Set("userID", parsed)
		c.Next()
	}, NewAdminHandler(nil).handleUploadChunk)

	uploadLegacyChunk(t, router, uploadID, 42, []byte("owner chunk"))
	uploadLegacyChunk(t, router, uploadID, 7, []byte("other user chunk"))

	ownerChunk, err := os.ReadFile(filepath.Join(storagePath, "legacy-chunks", "42", uploadID, "chunk-0"))
	if err != nil {
		t.Fatalf("read owner's chunk: %v", err)
	}
	if string(ownerChunk) != "owner chunk" {
		t.Fatalf("owner's chunk = %q, want %q", ownerChunk, "owner chunk")
	}
	otherChunk, err := os.ReadFile(filepath.Join(storagePath, "legacy-chunks", "7", uploadID, "chunk-0"))
	if err != nil {
		t.Fatalf("read other user's chunk: %v", err)
	}
	if string(otherChunk) != "other user chunk" {
		t.Fatalf("other user's chunk = %q, want %q", otherChunk, "other user chunk")
	}
}

func uploadLegacyChunk(t *testing.T, router http.Handler, uploadID string, userID uint, payload []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"upload_id":    uploadID,
		"chunk_index":  "0",
		"total_chunks": "1",
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	part, err := writer.CreateFormFile("file", "chunk-0")
	if err != nil {
		t.Fatalf("create chunk form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write chunk payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/apk/upload/chunk?user=%d", userID), &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("user %d upload status = %d, body %q", userID, recorder.Code, recorder.Body.String())
	}
}
