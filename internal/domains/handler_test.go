package domains

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/pagination"
	"google.golang.org/protobuf/proto"
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

func getDomainsPage(t *testing.T, r *gin.Engine, target string) *pb.DomainPage {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s, want 200", target, w.Code, w.Body.String())
	}
	page := &pb.DomainPage{}
	if err := proto.Unmarshal(w.Body.Bytes(), page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return page
}

func TestHandleGetDomainsPageWalksCursor(t *testing.T) {
	db := newDomainsTestDB(t)
	owner := seedOwner(t, db)
	seedDomains(t, db, owner.ID, "e.example", "a.example", "d.example", "b.example", "c.example")
	r := newDomainsTestRouter(db)

	var walked []string
	target := "/domains/page?limit=2"
	for pages := 0; ; pages++ {
		if pages > 3 {
			t.Fatal("cursor never reported the last page")
		}
		page := getDomainsPage(t, r, target)
		for _, d := range page.Data {
			if d.User.GetEmail() != owner.Email {
				t.Fatalf("domain %q owner = %q, want %q", d.Domain, d.User.GetEmail(), owner.Email)
			}
			walked = append(walked, d.Domain)
		}
		if !page.Page.GetHasMore() {
			if page.Page.GetNextCursor() != "" {
				t.Fatalf("last page carries next cursor %q", page.Page.GetNextCursor())
			}
			break
		}
		target = "/domains/page?limit=2&cursor=" + url.QueryEscape(page.Page.GetNextCursor())
	}
	if fmt.Sprint(walked) != "[a.example b.example c.example d.example e.example]" {
		t.Fatalf("walked %v, want a..e once each", walked)
	}
}

func TestHandleGetDomainsPageLimitAndCursorValidation(t *testing.T) {
	db := newDomainsTestDB(t)
	owner := seedOwner(t, db)
	names := make([]string, 0, defaultPageLimit+5)
	for i := 0; i < defaultPageLimit+5; i++ {
		names = append(names, fmt.Sprintf("d%02d.example", i))
	}
	seedDomains(t, db, owner.ID, names...)
	r := newDomainsTestRouter(db)

	for _, limit := range []string{"0", "-1", "101", "abc"} {
		if got := len(getDomainsPage(t, r, "/domains/page?limit="+limit).Data); got != defaultPageLimit {
			t.Errorf("limit=%s returned %d rows, want default %d", limit, got, defaultPageLimit)
		}
	}
	if got := len(getDomainsPage(t, r, "/domains/page?limit=100").Data); got != len(names) {
		t.Errorf("limit=100 returned %d rows, want %d", got, len(names))
	}

	badID, err := pagination.Encode("d00.example", "not-a-number")
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	wrongArity, err := pagination.Encode("d00.example")
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	for cursor, want := range map[string]string{
		"not base64!": `{"error":"invalid cursor"}`,
		wrongArity:    `{"error":"invalid cursor"}`,
		badID:         `{"error":"invalid cursor: bad id"}`,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/domains/page?cursor="+url.QueryEscape(cursor), nil))
		if w.Code != http.StatusBadRequest || w.Body.String() != want {
			t.Errorf("cursor %q = %d %s, want 400 %s", cursor, w.Code, w.Body.String(), want)
		}
	}
}
