package tournaments

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// scores365LeagueIDPrefix mirrors scores365.LeagueIDPrefix. It is
// duplicated rather than imported to keep this package's test
// dependencies to the domain it covers.
const scores365LeagueIDPrefix = 6_000_000_000

// newPageTestRouter builds a tournaments router backed by an in-memory
// SQLite DB holding n tournaments whose primary keys sit in the
// 365scores league namespace (6e9+), i.e. above math.MaxUint32.
func newPageTestRouter(t *testing.T, n int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Migrator().DropTable(&Tournament{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := db.AutoMigrate(&Tournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for i := 0; i < n; i++ {
		row := Tournament{
			Model: gorm.Model{ID: uint(scores365LeagueIDPrefix + i)},
			// Zero-padded so lexicographic slug order matches the
			// insertion order the assertions rely on.
			Name: fmt.Sprintf("Liga %03d", i),
			Slug: fmt.Sprintf("liga-%03d", i),
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}

	r := gin.New()
	NewHandler(NewRepository(db)).RegisterRoutes(r.Group("/"), HandlerDeps{
		AuthMiddleware: func(c *gin.Context) { c.Next() },
	})
	return r
}

func getPage(t *testing.T, r *gin.Engine, url string) *pb.TournamentPage {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("GET %s = %d, want 200 (body: %s)", url, w.Code, body)
	}
	var page pb.TournamentPage
	if err := proto.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal %s: %v", url, err)
	}
	return &page
}

// TestTournamentsPage_CursorWalksPastUint32IDs is the regression test
// for "invalid cursor: bad id" on /#/tournaments?page=3&size=20.
//
// The cursor carries the last row's primary key. Once the 365scores
// source began namespacing competitions at 6e9, server.ParseID's
// 32-bit parse rejected the cursor the server itself had just issued,
// so the admin table could not be paged past the first 6e9 row.
func TestTournamentsPage_CursorWalksPastUint32IDs(t *testing.T) {
	r := newPageTestRouter(t, 45)

	const size = 20
	seen := 0
	cursor := ""
	for page := 1; page <= 3; page++ {
		url := fmt.Sprintf("/tournaments/page?limit=%d", size)
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		got := getPage(t, r, url)

		wantLen := size
		if page == 3 {
			wantLen = 5
		}
		if len(got.Data) != wantLen {
			t.Fatalf("page %d returned %d rows, want %d", page, len(got.Data), wantLen)
		}
		seen += len(got.Data)

		wantMore := page < 3
		if got.Page.GetHasMore() != wantMore {
			t.Fatalf("page %d has_more = %v, want %v", page, got.Page.GetHasMore(), wantMore)
		}
		cursor = got.Page.GetNextCursor()
		if wantMore && cursor == "" {
			t.Fatalf("page %d has_more but no next_cursor", page)
		}
	}

	if seen != 45 {
		t.Fatalf("walked %d rows across 3 pages, want 45", seen)
	}
}

// TestTournamentsPage_RejectsGarbageCursor keeps the 400 path intact:
// widening ParseID must not turn a malformed cursor into a 200.
func TestTournamentsPage_RejectsGarbageCursor(t *testing.T) {
	r := newPageTestRouter(t, 5)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tournaments/page?cursor=not-base64!!", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("garbage cursor = %d, want 400", w.Code)
	}
}
