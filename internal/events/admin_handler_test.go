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

	recorder, response := getAdminEvents(t, NewAdminHandler(db), "/events?date=2026-07-17")

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

	recorder, response := getAdminEvents(t, NewAdminHandler(db), "/events")

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

	recorder, _ := getAdminEvents(t, NewAdminHandler(db), "/events?date=not-a-date")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestAdminEventsInvalidDateMentionsUTC(t *testing.T) {
	db := setupAdminHandlerTestDB(t)

	recorder, _ := getAdminEvents(t, NewAdminHandler(db), "/events?date=not-a-date")

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

	recorder, response := getAdminEventsPage(t, NewAdminHandler(db), "/events/page?limit=10&direction=desc")

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
	recorder, _ := getAdminEventsPage(t, NewAdminHandler(db), "/events/page?direction=sideways")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: want %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}
