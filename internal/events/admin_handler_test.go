package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func setupAdminHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&Team{}, &tournaments.Tournament{}, &Event{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func getAdminEvents(t *testing.T, handler *AdminHandler, target string) (*httptest.ResponseRecorder, *pb.EventsList) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)

	handler.handleGetEvents(ctx)

	response := &pb.EventsList{}
	if recorder.Code == http.StatusOK {
		if err := proto.Unmarshal(recorder.Body.Bytes(), response); err != nil {
			t.Fatalf("decode events response: %v", err)
		}
	}
	return recorder, response
}

func TestAdminEventsDateUsesUnixMilliseconds(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	day := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&Event{SofaScoreEventId: 1, StartTimestamp: day.Add(12 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}

	recorder, response := getAdminEvents(t, NewAdminHandler(NewRepository(db)), "/events?date=2026-07-17")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, recorder.Code)
	}
	if len(response.Data) != 1 {
		t.Fatalf("events: want 1, got %d", len(response.Data))
	}
}

func TestAdminEventsWithoutDateUsesUnixMilliseconds(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	if err := db.Create(&[]Event{
		{SofaScoreEventId: 1, StartTimestamp: time.Now().Add(-time.Hour).UnixMilli()},
		{SofaScoreEventId: 2, StartTimestamp: time.Now().Add(time.Hour).UnixMilli()},
	}).Error; err != nil {
		t.Fatalf("seed events: %v", err)
	}

	recorder, response := getAdminEvents(t, NewAdminHandler(NewRepository(db)), "/events")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, recorder.Code)
	}
	if len(response.Data) != 1 {
		t.Fatalf("events: want 1 future event, got %d", len(response.Data))
	}
	if response.Data[0].SofaScoreEventId != 2 {
		t.Fatalf("event: want SofaScore ID 2, got %d", response.Data[0].SofaScoreEventId)
	}
}

func TestAdminEventsRejectsInvalidDate(t *testing.T) {
	db := setupAdminHandlerTestDB(t)

	recorder, _ := getAdminEvents(t, NewAdminHandler(NewRepository(db)), "/events?date=not-a-date")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestAdminEventsInvalidDateMentionsUTC(t *testing.T) {
	db := setupAdminHandlerTestDB(t)

	recorder, _ := getAdminEvents(t, NewAdminHandler(NewRepository(db)), "/events?date=not-a-date")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	body := map[string]string{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if !strings.Contains(body["error"], "UTC") {
		t.Errorf("error message should mention UTC so callers know the day boundary is interpreted in UTC; got %q", body["error"])
	}
}

func getAdminEventsPage(t *testing.T, handler *AdminHandler, target string) (*httptest.ResponseRecorder, *pb.EventPage) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)

	handler.handleGetEventsPage(ctx)

	response := &pb.EventPage{}
	if recorder.Code == http.StatusOK {
		if err := proto.Unmarshal(recorder.Body.Bytes(), response); err != nil {
			t.Fatalf("decode events page response: %v", err)
		}
	}
	return recorder, response
}

func TestHandleGetEventsPage_DescendingOrder(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	now := time.Now().UnixMilli()
	for i, ts := range []int64{now, now + 3600_000, now + 7200_000} {
		if err := db.Create(&Event{
			SofaScoreEventId: int64(1000 + i),
			StartTimestamp:   ts,
			Sport:            "football",
			StatusType:       "notstarted",
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	recorder, response := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?limit=10&direction=desc")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d (%s)", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if len(response.Data) != 3 {
		t.Fatalf("want 3 events, got %d", len(response.Data))
	}
	if response.Data[0].StartTimestamp < response.Data[1].StartTimestamp {
		t.Errorf("expected descending order, got %d before %d", response.Data[0].StartTimestamp, response.Data[1].StartTimestamp)
	}
}

func TestHandleGetEventsPage_RejectsInvalidDirection(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	recorder, _ := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?direction=sideways")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestHandleGetEventsPage_DefaultsFromToTodayUTC(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	now := time.Now()
	if err := db.Create(&[]Event{
		{SofaScoreEventId: 4000, StartTimestamp: now.Add(-25 * time.Hour).UnixMilli(), Sport: "football", StatusType: "notstarted"},
		{SofaScoreEventId: 4001, StartTimestamp: now.Add(time.Hour).UnixMilli(), Sport: "football", StatusType: "notstarted"},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	recorder, response := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?limit=10")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	if len(response.Data) != 1 || response.Data[0].SofaScoreEventId != 4001 {
		t.Fatalf("want only the future event (id 4001), got %d events", len(response.Data))
	}
}

func TestHandleGetEventsPage_InvalidTZ_400(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	recorder, _ := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?tz=Mars/Olympus_Mons&limit=10")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "tz") {
		t.Errorf("error should mention 'tz', got %s", recorder.Body.String())
	}
}

func TestHandleGetEventsPage_InvalidStatus_400(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	recorder, _ := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?status=paused&limit=10")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", recorder.Code)
	}
}

func TestHandleGetEventsPage_MalformedFrom_400(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	recorder, _ := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)), "/events/page?from=not-a-date&limit=10")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", recorder.Code)
	}

	body := map[string]string{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if !strings.Contains(body["error"], "YYYY-MM-DD") {
		t.Errorf("error should explain the expected 'from' format; got %q", body["error"])
	}
	if strings.Contains(body["error"], "UTC") {
		t.Errorf("error should not claim 'from' is interpreted in UTC; got %q", body["error"])
	}
}

func TestHandleGetEventsPage_FromInUserTZ(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	// Midnight of 2026-08-27 in Pacific/Auckland (NZST, UTC+12) is 2026-08-26T12:00Z.
	// Interpreting the same date in UTC would push the boundary 12 hours later,
	// so an event inside that window is the discriminator.
	beforeAucklandMidnight := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	afterAucklandMidnight := time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC)
	if err := db.Create(&[]Event{
		{SofaScoreEventId: 6000, StartTimestamp: beforeAucklandMidnight.UnixMilli(), Sport: "football", StatusType: "notstarted"},
		{SofaScoreEventId: 6001, StartTimestamp: afterAucklandMidnight.UnixMilli(), Sport: "football", StatusType: "notstarted"},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	recorder, response := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)),
		"/events/page?from=2026-08-27&tz=Pacific/Auckland&limit=10")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	if len(response.Data) != 1 || response.Data[0].SofaScoreEventId != 6001 {
		ids := make([]int64, 0, len(response.Data))
		for _, e := range response.Data {
			ids = append(ids, e.SofaScoreEventId)
		}
		t.Fatalf("want only event 6001 ('from' parsed in Pacific/Auckland, not UTC), got %v", ids)
	}
}

func TestHandleGetEventsPage_FromInNonUTCBoundary(t *testing.T) {
	db := setupAdminHandlerTestDB(t)
	anchor := time.Date(2026, 8, 26, 3, 30, 0, 0, time.UTC)
	if err := db.Create(&Event{
		SofaScoreEventId: 5000, StartTimestamp: anchor.Add(-30 * time.Minute).UnixMilli(),
		Sport: "football", StatusType: "notstarted",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Create(&Event{
		SofaScoreEventId: 5001, StartTimestamp: anchor.Add(time.Hour).UnixMilli(),
		Sport: "football", StatusType: "notstarted",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	recorder, response := getAdminEventsPage(t, NewAdminHandler(NewRepository(db)),
		"/events/page?tz=America/Santo_Domingo&limit=10")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	if len(response.Data) != 1 || response.Data[0].SofaScoreEventId != 5001 {
		t.Fatalf("want only event 5001 (post-midnight in TZ), got %d events", len(response.Data))
	}
}
