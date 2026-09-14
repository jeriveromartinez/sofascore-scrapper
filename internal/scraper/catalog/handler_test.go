// internal/scraper/catalog/handler_test.go
package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	r, db := newTestRouter(t)
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

	// Fix C1 (PR #122): when the client omits "enabled", the created
	// league must default to enabled=true. Otherwise ActiveLeagues
	// would silently drop it from the scheduler. Verify directly
	// against the DB to dodge any client-side JSON tagging tricks.
	var got ScraperLeague
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("read back created league: %v", err)
	}
	if !got.Enabled {
		t.Errorf("default-enabled behaviour broken: created league has Enabled=false")
	}
}

// TestHandler_Create_ExplicitFalseStaysFalse is the inverse guard for
// fix C1 (PR #122). The default-enable fix must NOT swallow an
// explicit enabled:false — admins who explicitly disable a new league
// (e.g. a staging-only league) must continue to be able to opt out.
func TestHandler_Create_ExplicitFalseStaysFalse(t *testing.T) {
	r, db := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "Premier League", "country": "GB", "sport": "football",
		"enabled": false,
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var got ScraperLeague
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Enabled {
		t.Errorf("explicit enabled:false must be respected; got Enabled=true")
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

// TestHandler_Create_OverrideSourcePersisted covers Fix 6 (P2).
// The Create DTO now accepts override_source; operators use it to
// pin a FotMob league to the scores365 dispatcher and vice versa.
// We verify the value round-trips through the DB read-back.
func TestHandler_Create_OverrideSourcePersisted(t *testing.T) {
	r, db := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "Premier League", "country": "GB", "sport": "football",
		"override_source": "scores365",
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var got ScraperLeague
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.OverrideSource == nil || *got.OverrideSource != "scores365" {
		t.Errorf("OverrideSource = %v, want \"scores365\"", got.OverrideSource)
	}
}

// TestHandler_Update_OverrideSourceWhitelist covers Fix 6 (P2).
// Before the fix, PATCH /scraper-leagues/:id silently dropped
// override_source because the whitelist omitted it. After the
// fix the field round-trips into the DB row.
func TestHandler_Update_OverrideSourceWhitelist(t *testing.T) {
	r, db := newTestRouter(t)
	createBody, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "PL", "country": "GB", "sport": "football",
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status: %d body: %s", w.Code, w.Body.String())
	}
	var created ScraperLeague
	if err := db.First(&created).Error; err != nil {
		t.Fatalf("read back created: %v", err)
	}

	patchBody, _ := json.Marshal(map[string]any{"override_source": "scores365"})
	patchReq := httptest.NewRequest("PATCH", "/api/admin/v1/scraper-leagues/"+strconv.FormatUint(uint64(created.ID), 10), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	r.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusOK {
		t.Fatalf("update status: %d body: %s", patchW.Code, patchW.Body.String())
	}

	var updated ScraperLeague
	if err := db.First(&updated, created.ID).Error; err != nil {
		t.Fatalf("read back updated: %v", err)
	}
	if updated.OverrideSource == nil || *updated.OverrideSource != "scores365" {
		t.Errorf("after PATCH OverrideSource = %v, want \"scores365\"", updated.OverrideSource)
	}
}
