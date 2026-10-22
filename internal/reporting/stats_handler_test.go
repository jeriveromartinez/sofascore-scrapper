package reporting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func newStatsRouter(db *gorm.DB) *gin.Engine {
	router := gin.New()
	noAuth := func(c *gin.Context) { c.Next() }
	NewStatsHandler(NewRepository(db)).RegisterRoutes(router.Group("/api"), StatsHandlerDeps{AuthMiddleware: noAuth})
	return router
}

func getTopEvents(t *testing.T, router *gin.Engine, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/stats/top-events"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestStatsHandlerTopEventsLimit(t *testing.T) {
	db := newWindowTestDB(t)
	for i := 1; i <= 15; i++ {
		insertWindowLog(t, db, strconv.Itoa(1000+i), 2_000_000)
	}
	router := newStatsRouter(db)

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"default", "", 10},
		{"explicit", "?limit=3", 3},
		{"non numeric falls back to default", "?limit=abc", 10},
		{"zero falls back to default", "?limit=0", 10},
		{"negative falls back to default", "?limit=-5", 10},
		{"above cap is clamped by the repository", "?limit=500", 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := getTopEvents(t, router, tc.query)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
			}
			var resp pb.TopEventsResponse
			if err := proto.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got := len(resp.GetStats()); got != tc.want {
				t.Fatalf("stats=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestStatsHandlerTopEventsHidesDatabaseError(t *testing.T) {
	db := newWindowTestDB(t)
	if err := db.Migrator().DropTable("playback_logs"); err != nil {
		t.Fatalf("drop playback_logs: %v", err)
	}
	router := newStatsRouter(db)

	w := getTopEvents(t, router, "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusInternalServerError)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "internal server error" {
		t.Fatalf("error=%q, want the generic message", resp["error"])
	}
	if strings.Contains(w.Body.String(), "playback_logs") {
		t.Fatalf("response leaks database internals: %s", w.Body.String())
	}
}
