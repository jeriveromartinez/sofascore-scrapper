package tournaments

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// globalConfigDBCounter hands out the name of the in-memory database each
// call gets. A shared-cache database survives until its last connection
// closes, so the name has to be unique per call.
var globalConfigDBCounter atomic.Int64

// newGlobalConfigTestDB opens an in-memory DB holding the tournaments and
// the global-config rows the admin page works with.
func newGlobalConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// The name is unique per call so a database left behind by an earlier
	// run cannot be picked up again, and the pool is closed so the database
	// does not outlive the test.
	dsn := fmt.Sprintf("file:gcfg-%d?mode=memory&cache=shared", globalConfigDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, m := range []any{&GlobalTournamentConfig{}, &Tournament{}} {
		if err := db.Migrator().DropTable(m); err != nil {
			t.Fatalf("drop table: %v", err)
		}
	}
	if err := db.AutoMigrate(&Tournament{}, &GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestNewGlobalConfigTestDBUsesAFreshDatabasePerCall pins the hermeticity of
// the in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one wrote to.
func TestNewGlobalConfigTestDBUsesAFreshDatabasePerCall(t *testing.T) {
	first := newGlobalConfigTestDB(t)
	if err := first.Exec("CREATE TABLE marker (id INTEGER)").Error; err != nil {
		t.Fatalf("create marker table: %v", err)
	}

	second := newGlobalConfigTestDB(t)
	if second.Migrator().HasTable("marker") {
		t.Fatal("the second call reused the database of the first one")
	}
}

// newGlobalConfigTestRouter wires the tournaments list endpoint and the
// global-config endpoints against one in-memory DB, so a test can walk
// the exact path the admin page takes: read the tournaments, then post
// back the ids it read.
func newGlobalConfigTestRouter(t *testing.T, ids []uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := newGlobalConfigTestDB(t)

	for i, id := range ids {
		row := Tournament{
			Model: gorm.Model{ID: id},
			Name:  fmt.Sprintf("Liga %03d", i),
			Slug:  fmt.Sprintf("liga-%03d", i),
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed tournament %d: %v", id, err)
		}
	}

	r := gin.New()
	g := r.Group("/")
	pass := func(c *gin.Context) { c.Next() }
	NewHandler(NewRepository(db)).RegisterRoutes(g, HandlerDeps{AuthMiddleware: pass})
	NewGlobalConfigHandler(NewGlobalConfigRepository(db)).RegisterRoutes(g, GlobalConfigHandlerDeps{AuthMiddleware: pass})
	return r
}

// TestGlobalConfig_KeepsIDsAboveUint32 is the regression test for
// "Error 1452 ... foreign key constraint fails" on /#/global-config.
//
// Tournament ids are namespaced at 6_000_000_000, above math.MaxUint32.
// While the proto carried them as uint32 the server truncated every id
// it sent to the admin page; the page posted those truncated ids back,
// and the insert into global_tournament_configs had no parent row to
// point at, so MariaDB rejected it with error 1452.
func TestGlobalConfig_KeepsIDsAboveUint32(t *testing.T) {
	want := []uint{scores365LeagueIDPrefix + 352, scores365LeagueIDPrefix + 7271, 7_000_097_718}
	r := newGlobalConfigTestRouter(t, want)

	// 1. The admin page lists the tournaments.
	listed := getPage(t, r, "/tournaments/page?limit=50")
	if len(listed.Data) != len(want) {
		t.Fatalf("listed %d tournaments, want %d", len(listed.Data), len(want))
	}
	ids := make([]uint64, 0, len(listed.Data))
	for _, row := range listed.Data {
		if row.GetId() <= math.MaxUint32 {
			t.Fatalf("tournament %q came back as id %d, truncated below MaxUint32", row.GetSlug(), row.GetId())
		}
		ids = append(ids, row.GetId())
	}

	// 2. The user selects them and the page posts the ids straight back.
	body, err := proto.Marshal(&pb.SetTournamentIdsRequest{TournamentIds: ids})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/global-tournament-config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		got, _ := io.ReadAll(w.Body)
		t.Fatalf("POST /global-tournament-config = %d, want 201 (body: %s)", w.Code, got)
	}

	// 3. Every stored config must point at a tournament that exists.
	saved := &pb.GlobalTournamentConfigList{}
	if err := proto.Unmarshal(w.Body.Bytes(), saved); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(saved.Configs) != len(want) {
		t.Fatalf("saved %d configs, want %d", len(saved.Configs), len(want))
	}
	for i, cfg := range saved.Configs {
		if cfg.GetTournamentId() != uint64(want[i]) {
			t.Fatalf("config %d stored tournament_id %d, want %d", i, cfg.GetTournamentId(), want[i])
		}
	}
}
