package tournaments

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

const genericInternalErrorBody = `{"error":"internal server error"}`

func TestHandlersHideDatabaseErrorsBehindGenericMessage(t *testing.T) {
	db := newAssignmentsTestDB(t, 1, 1)
	if err := db.AutoMigrate(&GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate global config: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/")
	pass := func(c *gin.Context) { c.Next() }
	NewHandler(NewRepository(db)).RegisterRoutes(g, HandlerDeps{AuthMiddleware: pass})
	NewDeviceAssignmentsHandler(NewDeviceAssignmentsRepository(db)).RegisterRoutes(g, DeviceAssignmentsHandlerDeps{AuthMiddleware: pass})
	NewGlobalConfigHandler(NewGlobalConfigRepository(db)).RegisterRoutes(g, GlobalConfigHandlerDeps{AuthMiddleware: pass})

	for _, table := range []any{&GlobalTournamentConfig{}, &DeviceTournament{}, &Tournament{}} {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Fatalf("drop table: %v", err)
		}
	}

	pair := &pb.AssignTournamentRequest{DeviceId: 1, TournamentId: 1}
	ids := &pb.SetTournamentIdsRequest{TournamentIds: []uint64{1}}
	cases := []struct {
		method, url string
		body        proto.Message
	}{
		{http.MethodGet, "/tournaments", nil},
		{http.MethodGet, "/tournaments/page", nil},
		{http.MethodPost, "/tournaments", &pb.TournamentRequest{Name: "Cup", Slug: "cup"}},
		{http.MethodPut, "/tournaments/1", &pb.TournamentRequest{Name: "Cup", Slug: "cup"}},
		{http.MethodDelete, "/tournaments/1", nil},
		{http.MethodGet, "/device-tournaments", nil},
		{http.MethodGet, "/device-tournaments/page", nil},
		{http.MethodGet, "/device-tournaments/1", nil},
		{http.MethodPost, "/device-tournaments", pair},
		{http.MethodDelete, "/device-tournaments", pair},
		{http.MethodPut, "/device-tournaments/1", ids},
		{http.MethodGet, "/global-tournament-config", nil},
		{http.MethodPost, "/global-tournament-config", ids},
		{http.MethodDelete, "/global-tournament-config/1", nil},
	}
	for _, tc := range cases {
		w := doProto(t, r, tc.method, tc.url, tc.body)
		if w.Code != http.StatusInternalServerError || w.Body.String() != genericInternalErrorBody {
			t.Errorf("%s %s = %d %s, want 500 %s", tc.method, tc.url, w.Code, w.Body.String(), genericInternalErrorBody)
		}
	}
}
