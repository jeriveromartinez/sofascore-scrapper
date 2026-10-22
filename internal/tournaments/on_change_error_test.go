package tournaments

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestOnChangeFailureIsLoggedAndWriteStillSucceeds(t *testing.T) {
	db := newAssignmentsTestDB(t, 1, 1)
	if err := db.AutoMigrate(&GlobalTournamentConfig{}); err != nil {
		t.Fatalf("migrate global config: %v", err)
	}
	logs := captureDefaultLogger(t)
	failingOnChange := func(context.Context) error { return errors.New("cache publish exploded") }

	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/")
	pass := func(c *gin.Context) { c.Next() }
	assignments := NewDeviceAssignmentsHandler(NewDeviceAssignmentsRepository(db))
	assignments.SetOnChange(failingOnChange)
	assignments.RegisterRoutes(g, DeviceAssignmentsHandlerDeps{AuthMiddleware: pass})
	globalConfig := NewGlobalConfigHandler(NewGlobalConfigRepository(db))
	globalConfig.SetOnChange(failingOnChange)
	globalConfig.RegisterRoutes(g, GlobalConfigHandlerDeps{AuthMiddleware: pass})

	pair := &pb.AssignTournamentRequest{DeviceId: 1, TournamentId: 1}
	ids := &pb.SetTournamentIdsRequest{TournamentIds: []uint64{1}}
	cases := []struct {
		method, url string
		body        proto.Message
		wantStatus  int
	}{
		{http.MethodPost, "/device-tournaments", pair, http.StatusCreated},
		{http.MethodDelete, "/device-tournaments", pair, http.StatusOK},
		{http.MethodPut, "/device-tournaments/1", ids, http.StatusOK},
		{http.MethodPost, "/global-tournament-config", ids, http.StatusCreated},
		{http.MethodDelete, "/global-tournament-config/1", nil, http.StatusOK},
	}
	for _, tc := range cases {
		logs.Reset()
		w := doProto(t, r, tc.method, tc.url, tc.body)
		if w.Code != tc.wantStatus {
			t.Errorf("%s %s = %d %s, want %d", tc.method, tc.url, w.Code, w.Body.String(), tc.wantStatus)
		}
		if got := logs.String(); !strings.Contains(got, "level=ERROR") || !strings.Contains(got, "cache publish exploded") {
			t.Errorf("%s %s: onChange failure not logged, log output: %q", tc.method, tc.url, got)
		}
	}
}
