// internal/scraper/catalog/handler_test.go
package catalog

import (
	"bytes"
	"context"
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
// The Create DTO accepts override_source; the value round-trips
// through the DB read-back.
//
// The override must name the row's own source: a league id only
// means something inside the source that publishes it, so pinning a
// FotMob league to the scores365 dispatcher would look for a
// competition the id does not belong to.
func TestHandler_Create_OverrideSourcePersisted(t *testing.T) {
	r, db := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "Premier League", "country": "GB", "sport": "football",
		"override_source": "fotmob",
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
	if got.OverrideSource == nil || *got.OverrideSource != "fotmob" {
		t.Errorf("OverrideSource = %v, want \"fotmob\"", got.OverrideSource)
	}
}

// TestHandler_Create_CrossSourceOverrideRejected is the regression
// test for the namespace crossing: a row whose override names another
// source would be looked up in that source's league id space, where
// the number identifies a different league. The API refuses it.
func TestHandler_Create_CrossSourceOverrideRejected(t *testing.T) {
	r, db := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "Premier League", "country": "GB", "sport": "football",
		"override_source": "scores365",
	})
	req := httptest.NewRequest("POST", "/api/admin/v1/scraper-leagues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var count int64
	if err := db.Model(&ScraperLeague{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("rejected league must not be stored, found %d rows", count)
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

	patchBody, _ := json.Marshal(map[string]any{"override_source": "fotmob"})
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
	if updated.OverrideSource == nil || *updated.OverrideSource != "fotmob" {
		t.Errorf("after PATCH OverrideSource = %v, want \"fotmob\"", updated.OverrideSource)
	}
}

// TestHandler_Update_CrossSourceOverrideRejected is the update-side
// counterpart of the create-side namespace guard: pinning a league to
// another source is refused instead of silently never matching.
func TestHandler_Update_CrossSourceOverrideRejected(t *testing.T) {
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
	if patchW.Code != http.StatusBadRequest {
		t.Fatalf("update status: %d body: %s", patchW.Code, patchW.Body.String())
	}

	var updated ScraperLeague
	if err := db.First(&updated, created.ID).Error; err != nil {
		t.Fatalf("read back updated: %v", err)
	}
	if updated.OverrideSource != nil {
		t.Errorf("rejected override must not be stored, got %v", *updated.OverrideSource)
	}
}

// TestHandler_Update_ClearingOverrideKeepsLeagueActive is the
// regression test for the un-normalised override write.
//
// The service normalised the override for the validation but persisted
// the raw wire value, so clearing the override stored an empty
// string. The catalog only dispatches rows whose override is NULL or
// equal to their own source, so the league silently dropped out of
// the scrape.
func TestHandler_Update_ClearingOverrideKeepsLeagueActive(t *testing.T) {
	r, db := newTestRouter(t)
	createBody, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "PL", "country": "GB", "sport": "football",
		"override_source": "fotmob",
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

	patchBody, _ := json.Marshal(map[string]any{"override_source": ""})
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
	if updated.OverrideSource != nil {
		t.Errorf("cleared override must be stored as NULL, got %q", *updated.OverrideSource)
	}

	active, err := NewRepository(db).ActiveLeagues(context.Background())
	if err != nil {
		t.Fatalf("active leagues: %v", err)
	}
	if len(active) != 1 || active[0].SourceLeagueId != "47" {
		t.Fatalf("the league must stay dispatchable, got %+v", active)
	}
}

// TestHandler_Update_EmptyNameRejected is the regression test for the
// validation gap: the patched values were never merged into the row
// the validator saw, so a patch that emptied a required field passed
// validation and was stored.
func TestHandler_Update_EmptyNameRejected(t *testing.T) {
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

	patchBody, _ := json.Marshal(map[string]any{"name": "   "})
	patchReq := httptest.NewRequest("PATCH", "/api/admin/v1/scraper-leagues/"+strconv.FormatUint(uint64(created.ID), 10), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	r.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusBadRequest {
		t.Fatalf("update status: %d body: %s", patchW.Code, patchW.Body.String())
	}

	var updated ScraperLeague
	if err := db.First(&updated, created.ID).Error; err != nil {
		t.Fatalf("read back updated: %v", err)
	}
	if updated.Name != "PL" {
		t.Errorf("the rejected patch must not be stored, name = %q", updated.Name)
	}
}

// TestHandler_Update_WrongFieldTypeRejected makes sure a patch with a
// value of the wrong type is refused instead of being written into the
// column as-is.
func TestHandler_Update_WrongFieldTypeRejected(t *testing.T) {
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

	patchBody, _ := json.Marshal(map[string]any{"enabled": "yes"})
	patchReq := httptest.NewRequest("PATCH", "/api/admin/v1/scraper-leagues/"+strconv.FormatUint(uint64(created.ID), 10), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	r.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusBadRequest {
		t.Fatalf("update status: %d body: %s", patchW.Code, patchW.Body.String())
	}

	var updated ScraperLeague
	if err := db.First(&updated, created.ID).Error; err != nil {
		t.Fatalf("read back updated: %v", err)
	}
	if !updated.Enabled {
		t.Error("the rejected patch must not have changed the enabled flag")
	}
}

// TestHandler_Create_LongCountryTruncated is the regression test for
// the country column width. scraper_leagues.country is a size:8
// column and MariaDB runs in strict mode, so a longer label ("United
// Kingdom") makes the INSERT fail with "Data too long" and the whole
// admin request errors out.
func TestHandler_Create_LongCountryTruncated(t *testing.T) {
	r, db := newTestRouter(t)
	body, _ := json.Marshal(map[string]any{
		"source": "fotmob", "source_league_id": "47", "name": "PL", "country": "United Kingdom", "sport": "football",
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
		t.Fatalf("read back created league: %v", err)
	}
	if len([]rune(got.Country)) > countryColumnSize {
		t.Errorf("country %q does not fit the %d character column", got.Country, countryColumnSize)
	}
}
