package playback

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

func assertGenericInternalError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", w.Code, w.Body.String())
	}
	if body := w.Body.String(); strings.Contains(body, "playback_logs") || strings.Contains(body, "no such table") {
		t.Fatalf("500 body leaks the underlying error: %s", body)
	}
}

func TestPlaybackHandlersDoNotLeakInternalErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	dev := devices.Device{Token: "leak-test"}
	if err := db.Create(&dev).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if err := db.Migrator().DropTable(&PlaybackLog{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	repo := NewRepository(db)

	router := gin.New()
	NewAdminHandler(repo).RegisterRoutes(router.Group(""), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})
	for _, path := range []string{"/playback", "/playback/page"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			assertGenericInternalError(t, w)
		})
	}

	t.Run("report viewing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(mustMarshal(t, &pb.LogPlaybackRequest{Content: "ch1"})))
		c.Set("device", dev)
		NewAppHandler(NewService(repo)).handleReportViewing(c)
		assertGenericInternalError(t, w)
	})
}
