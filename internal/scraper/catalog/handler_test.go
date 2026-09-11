// internal/scraper/catalog/handler_test.go
package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&ScraperLeague{})
	repo := NewRepository(db)
	svc := NewService(repo, nil)
	h := NewHandler(svc)
	r := gin.New()
	g := r.Group("/api/admin/v1")
	h.RegisterRoutes(g)
	return r, db
}

func TestHandler_Create_Valid(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "Premier League", "country": "GB", "sport": "football",
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Create_DuplicateReturns409(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "X",
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	req2 := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w2.Code)
	}
}

func TestHandler_Create_MissingFieldsReturns400(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{"source": "fotmob"})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
