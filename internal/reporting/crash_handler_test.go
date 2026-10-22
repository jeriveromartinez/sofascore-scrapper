package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCrashHandlerReturnsStableInvalidJSONError(t *testing.T) {
	router := gin.New()
	NewCrashHandler(nil).RegisterRoutes(router.Group("/api/app/v1"))
	req := httptest.NewRequest(http.MethodPost, "/api/app/v1/crash-report", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] != "invalid crash report" {
		t.Fatalf("error=%q, want stable error", response["error"])
	}
}

func newCrashTestRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:reporting-crash-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&CrashReport{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), db
}

// TestCrashHandlerHonoursRequestContext asserts the insert runs on the
// request ctx, so a client that went away does not keep a DB write going.
func TestCrashHandlerHonoursRequestContext(t *testing.T) {
	repo, db := newCrashTestRepo(t)
	router := gin.New()
	NewCrashHandler(repo).RegisterRoutes(router.Group("/api/app/v1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/app/v1/crash-report", strings.NewReader(`{"fatal":true,"error":"boom"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusInternalServerError)
	}
	var saved int64
	if err := db.Model(&CrashReport{}).Count(&saved).Error; err != nil {
		t.Fatalf("count crash reports: %v", err)
	}
	if saved != 0 {
		t.Fatalf("crash reports = %d, want 0 (the request ctx was cancelled)", saved)
	}
}

func TestCrashHandlerSavesReport(t *testing.T) {
	repo, db := newCrashTestRepo(t)
	router := gin.New()
	NewCrashHandler(repo).RegisterRoutes(router.Group("/api/app/v1"))

	req := httptest.NewRequest(http.MethodPost, "/api/app/v1/crash-report", strings.NewReader(`{"fatal":true,"error":"boom"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	var saved CrashReport
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("crash report not saved: %v", err)
	}
	if !saved.Fatal || saved.Error != "boom" {
		t.Fatalf("saved report = %+v", saved)
	}
}
