package events

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

func TestEventsAdminPageDoesNotLeakInternalErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminHandlerTestDB(t)
	if err := db.Migrator().DropTable(&Event{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	router := gin.New()
	NewAdminHandler(NewRepository(db)).RegisterRoutes(router.Group(""), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/events/page", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", w.Code, w.Body.String())
	}
	if b := w.Body.String(); strings.Contains(b, "events") || strings.Contains(b, "no such table") {
		t.Fatalf("500 body leaks the underlying error: %s", b)
	}
}
