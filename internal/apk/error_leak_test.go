package apk

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func apkMultipartRequest(t *testing.T, path string, fields map[string]string, fileName string, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if fileName != "" {
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func newLeakTestAdminRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	if err := db.Migrator().DropTable(&ApkVersion{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	handler := NewAdminHandler(NewRepository(db))
	handler.parseAPKInfo = func(string) (*APKInfo, error) {
		return &APKInfo{PackageName: "com.example.app", VersionName: "1.0.0", VersionCode: 7}, nil
	}
	router := gin.New()
	handler.RegisterRoutes(router.Group(""), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) {
		c.Set("userID", uint(42))
		c.Next()
	}})
	return router
}

func assertNoDatabaseLeak(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	body := w.Body.String()
	if strings.Contains(body, "no such table") || strings.Contains(body, "apk_versions") {
		t.Fatalf("response leaks the database error: %s", body)
	}
	if !strings.Contains(body, "could not save APK version") {
		t.Fatalf("body = %q, want the fixed message", body)
	}
}

func TestAdminUploadConflictDoesNotLeakDatabaseError(t *testing.T) {
	t.Setenv("APK_STORAGE_PATH", t.TempDir())
	router := newLeakTestAdminRouter(t)

	req := apkMultipartRequest(t, "/apk/upload", nil, "app.apk", []byte("apk bytes"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s, want 409", w.Code, w.Body.String())
	}
	assertNoDatabaseLeak(t, w)
}

func TestAdminAssembleConflictDoesNotLeakDatabaseError(t *testing.T) {
	t.Setenv("APK_STORAGE_PATH", t.TempDir())
	router := newLeakTestAdminRouter(t)
	uploadID := uuid.NewString()

	chunkReq := apkMultipartRequest(t, "/apk/upload/chunk", map[string]string{
		"upload_id":    uploadID,
		"chunk_index":  "0",
		"total_chunks": "1",
	}, "chunk-0", []byte("apk bytes"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, chunkReq)
	if w.Code != http.StatusOK {
		t.Fatalf("chunk status = %d body = %s, want 200", w.Code, w.Body.String())
	}

	assembleReq := apkMultipartRequest(t, "/apk/upload/assemble", map[string]string{
		"upload_id":    uploadID,
		"total_chunks": "1",
	}, "", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, assembleReq)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s, want 409", w.Code, w.Body.String())
	}
	assertNoDatabaseLeak(t, w)
}

func TestUploadHandlerDoesNotLeakInternalErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewUploadService(NewUploadStateStore(nil), NewChunkStore(t.TempDir()), nil, nil)
	handler := NewUploadHandler(service, nil)
	router := gin.New()
	handler.RegisterRoutes(router.Group(""), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) {
		c.Set("userID", uint(42))
		c.Next()
	}})

	uploadID := uuid.NewString()
	cases := []struct {
		name, method, path, body, want string
	}{
		{"status", http.MethodGet, "/apk/uploads/" + uploadID, "", "could not load upload status"},
		{"chunk", http.MethodPut, "/apk/uploads/" + uploadID + "/chunks/0", "chunk", "could not store chunk"},
		{"complete", http.MethodPost, "/apk/uploads/" + uploadID + "/complete", "", "could not complete upload"},
		{"abort", http.MethodDelete, "/apk/uploads/" + uploadID, "", "could not abort upload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code < 400 {
				t.Fatalf("status = %d, want a client error", w.Code)
			}
			if b := w.Body.String(); strings.Contains(b, "redis unavailable") {
				t.Fatalf("response leaks the internal error: %s", b)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("body = %q, want %q", w.Body.String(), tc.want)
			}
		})
	}
}
