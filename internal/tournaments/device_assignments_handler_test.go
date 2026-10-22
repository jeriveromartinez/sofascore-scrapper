package tournaments

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/pagination"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func newAssignmentsTestDB(t *testing.T, deviceCount, tournamentCount int) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &devices.Device{}, &Tournament{}, &DeviceTournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := 1; i <= deviceCount; i++ {
		if err := db.Create(&devices.Device{Model: gorm.Model{ID: uint(i)}, Token: fmt.Sprintf("tok-%d", i)}).Error; err != nil {
			t.Fatalf("seed device %d: %v", i, err)
		}
	}
	for i := 1; i <= tournamentCount; i++ {
		row := Tournament{Model: gorm.Model{ID: uint(i)}, Name: fmt.Sprintf("Liga %d", i), Slug: fmt.Sprintf("liga-%d", i)}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed tournament %d: %v", i, err)
		}
	}
	return db
}

func newAssignmentsTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewDeviceAssignmentsHandler(NewDeviceAssignmentsRepository(db)).RegisterRoutes(r.Group("/"), DeviceAssignmentsHandlerDeps{
		AuthMiddleware: func(c *gin.Context) { c.Next() },
	})
	return r
}

func doProto(t *testing.T, r *gin.Engine, method, url string, msg proto.Message) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if msg != nil {
		var err error
		body, err = proto.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestDeviceAssignments_ReassignAfterRemoveSucceeds(t *testing.T) {
	db := newAssignmentsTestDB(t, 1, 1)
	r := newAssignmentsTestRouter(db)
	pair := &pb.AssignTournamentRequest{DeviceId: 1, TournamentId: 1}

	if w := doProto(t, r, http.MethodPost, "/device-tournaments", pair); w.Code != http.StatusCreated {
		t.Fatalf("first assign = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}
	if w := doProto(t, r, http.MethodDelete, "/device-tournaments", pair); w.Code != http.StatusOK {
		t.Fatalf("remove = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	if w := doProto(t, r, http.MethodPost, "/device-tournaments", pair); w.Code != http.StatusCreated {
		t.Fatalf("re-assign = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}

	var rows int64
	if err := db.Unscoped().Model(&DeviceTournament{}).Count(&rows).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("device_tournaments rows incl. soft-deleted = %d, want 1", rows)
	}
}

func assignedPairs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var rows []DeviceTournament
	if err := db.Order("device_id ASC, tournament_id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load assignments: %v", err)
	}
	pairs := make([]string, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, fmt.Sprintf("%d-%d", row.DeviceID, row.TournamentID))
	}
	return pairs
}

func TestDeviceAssignments_AssignRemoveSet(t *testing.T) {
	db := newAssignmentsTestDB(t, 2, 3)
	changes := 0
	h := NewDeviceAssignmentsHandler(NewDeviceAssignmentsRepository(db))
	h.SetOnChange(func(context.Context) error { changes++; return nil })
	r := gin.New()
	h.RegisterRoutes(r.Group("/"), DeviceAssignmentsHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})

	w := doProto(t, r, http.MethodPost, "/device-tournaments", &pb.AssignTournamentRequest{DeviceId: 1, TournamentId: 2})
	if w.Code != http.StatusCreated {
		t.Fatalf("assign = %d %s, want 201", w.Code, w.Body.String())
	}
	created := &pb.DeviceTournament{}
	if err := proto.Unmarshal(w.Body.Bytes(), created); err != nil {
		t.Fatalf("decode assign response: %v", err)
	}
	if created.GetDeviceId() != 1 || created.GetTournamentId() != 2 {
		t.Fatalf("assign response = %d-%d, want 1-2", created.GetDeviceId(), created.GetTournamentId())
	}
	if w := doProto(t, r, http.MethodPost, "/device-tournaments", &pb.AssignTournamentRequest{DeviceId: 2, TournamentId: 1}); w.Code != http.StatusCreated {
		t.Fatalf("second assign = %d %s, want 201", w.Code, w.Body.String())
	}

	if w := doProto(t, r, http.MethodPut, "/device-tournaments/1", &pb.SetTournamentIdsRequest{TournamentIds: []uint64{1, 3}}); w.Code != http.StatusOK {
		t.Fatalf("set = %d %s, want 200", w.Code, w.Body.String())
	}
	if got := fmt.Sprint(assignedPairs(t, db)); got != "[1-1 1-3 2-1]" {
		t.Fatalf("after set: %s, want [1-1 1-3 2-1]", got)
	}

	if w := doProto(t, r, http.MethodDelete, "/device-tournaments", &pb.AssignTournamentRequest{DeviceId: 1, TournamentId: 3}); w.Code != http.StatusOK {
		t.Fatalf("remove = %d %s, want 200", w.Code, w.Body.String())
	}
	if w := doProto(t, r, http.MethodPut, "/device-tournaments/2", &pb.SetTournamentIdsRequest{}); w.Code != http.StatusOK {
		t.Fatalf("set empty = %d %s, want 200", w.Code, w.Body.String())
	}
	if got := fmt.Sprint(assignedPairs(t, db)); got != "[1-1]" {
		t.Fatalf("after remove and empty set: %s, want [1-1]", got)
	}

	w = doProto(t, r, http.MethodGet, "/device-tournaments/1", nil)
	list := &pb.DeviceTournamentList{}
	if err := proto.Unmarshal(w.Body.Bytes(), list); err != nil || w.Code != http.StatusOK {
		t.Fatalf("get by device = %d (decode err %v)", w.Code, err)
	}
	if len(list.DeviceTournaments) != 1 || list.DeviceTournaments[0].GetTournament().GetSlug() != "liga-1" {
		t.Fatalf("get by device returned %v, want the preloaded liga-1 assignment", list.DeviceTournaments)
	}

	if changes != 5 {
		t.Fatalf("onChange calls = %d, want 5 (one per successful write)", changes)
	}

	for _, tc := range []struct {
		method, url string
		body        proto.Message
		wantBody    string
	}{
		{http.MethodPut, "/device-tournaments/abc", &pb.SetTournamentIdsRequest{}, `{"error":"invalid device id"}`},
		{http.MethodGet, "/device-tournaments/abc", nil, `{"error":"invalid device id"}`},
	} {
		if w := doProto(t, r, tc.method, tc.url, tc.body); w.Code != http.StatusBadRequest || w.Body.String() != tc.wantBody {
			t.Errorf("%s %s = %d %s, want 400 %s", tc.method, tc.url, w.Code, w.Body.String(), tc.wantBody)
		}
	}
}

func getAssignmentsPage(t *testing.T, r *gin.Engine, target string) *pb.DeviceTournamentPage {
	t.Helper()
	w := doProto(t, r, http.MethodGet, target, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s, want 200", target, w.Code, w.Body.String())
	}
	page := &pb.DeviceTournamentPage{}
	if err := proto.Unmarshal(w.Body.Bytes(), page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return page
}

func TestDeviceAssignments_PageCursorRoundTrip(t *testing.T) {
	db := newAssignmentsTestDB(t, 3, 3)
	repo := NewDeviceAssignmentsRepository(db)
	// Inserted out of key order so the walk proves the
	// (device_id, tournament_id, id) ordering, not insertion order.
	for _, pair := range [][2]uint{{3, 1}, {1, 2}, {2, 3}, {1, 1}, {2, 1}, {3, 3}, {1, 3}} {
		if _, err := repo.Assign(context.Background(), pair[0], pair[1]); err != nil {
			t.Fatalf("seed %v: %v", pair, err)
		}
	}
	r := newAssignmentsTestRouter(db)

	var walked []string
	target := "/device-tournaments/page?limit=3"
	for pages := 0; ; pages++ {
		if pages > 3 {
			t.Fatal("cursor never reported the last page")
		}
		page := getAssignmentsPage(t, r, target)
		for _, row := range page.Data {
			walked = append(walked, fmt.Sprintf("%d-%d", row.GetDeviceId(), row.GetTournamentId()))
		}
		if !page.Page.GetHasMore() {
			if page.Page.GetNextCursor() != "" {
				t.Fatalf("last page carries next cursor %q", page.Page.GetNextCursor())
			}
			break
		}
		target = "/device-tournaments/page?limit=3&cursor=" + url.QueryEscape(page.Page.GetNextCursor())
	}
	if got := fmt.Sprint(walked); got != "[1-1 1-2 1-3 2-1 2-3 3-1 3-3]" {
		t.Fatalf("walked %s, want every pair once in key order", got)
	}

	if got := len(getAssignmentsPage(t, r, "/device-tournaments/page?limit=101").Data); got != 7 {
		t.Errorf("limit=101 fell back to %d rows, want all 7 (default limit 20)", got)
	}

	for cursor, want := range map[string]string{
		"not base64!":                      `{"error":"invalid cursor"}`,
		mustEncodeCursor(t, "1", "1"):      `{"error":"invalid cursor"}`,
		mustEncodeCursor(t, "x", "1", "1"): `{"error":"invalid cursor: bad device_id"}`,
		mustEncodeCursor(t, "1", "x", "1"): `{"error":"invalid cursor: bad tournament_id"}`,
		mustEncodeCursor(t, "1", "1", "x"): `{"error":"invalid cursor: bad id"}`,
	} {
		w := doProto(t, r, http.MethodGet, "/device-tournaments/page?cursor="+url.QueryEscape(cursor), nil)
		if w.Code != http.StatusBadRequest || w.Body.String() != want {
			t.Errorf("cursor %q = %d %s, want 400 %s", cursor, w.Code, w.Body.String(), want)
		}
	}
}

func mustEncodeCursor(t *testing.T, keys ...string) string {
	t.Helper()
	cursor, err := pagination.Encode(keys...)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	return cursor
}
