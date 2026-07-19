package reporting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
