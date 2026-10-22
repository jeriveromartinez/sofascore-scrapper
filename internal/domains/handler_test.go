package domains

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const genericInternalErrorBody = `{"error":"internal server error"}`

func newDomainsTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(NewRepository(db)).RegisterRoutes(r.Group("/"), HandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})
	return r
}

func TestHandlersHideDatabaseErrorsBehindGenericMessage(t *testing.T) {
	db := newDomainsTestDB(t)
	r := newDomainsTestRouter(db)
	if err := db.Migrator().DropTable(&Domain{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	for _, tc := range []struct{ method, url string }{
		{http.MethodGet, "/domains"},
		{http.MethodGet, "/domains/page"},
		{http.MethodDelete, "/domains/1"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.url, nil))
		if w.Code != http.StatusInternalServerError || w.Body.String() != genericInternalErrorBody {
			t.Errorf("%s %s = %d %s, want 500 %s", tc.method, tc.url, w.Code, w.Body.String(), genericInternalErrorBody)
		}
	}
}
