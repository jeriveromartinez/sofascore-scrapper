package push

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const pushesPath = "/api/web/v1/pushes"

func newPushTestRouter(f *timerFixture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(f.serviceWithPusher(newFakePusher()), HandlerDeps{
		CallerID: func(*gin.Context) (uint, bool) { return f.uid, true },
	})
	h.RegisterRoutes(r.Group("/api/web/v1"), func(*gin.Context) {})
	return r
}

func doPushRequest(t *testing.T, router *gin.Engine, method, path string, body proto.Message) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := proto.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodePushResponse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, out proto.Message) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%q)", w.Code, wantStatus, w.Body.String())
	}
	if err := proto.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
}

func assertGenericInternalError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%q)", w.Code, w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"error":"internal server error"}` {
		t.Fatalf("body = %s, want the generic internal error", got)
	}
}

// TestHandler_InternalErrorsDoNotLeakDetails breaks the schema under
// every read endpoint and checks the 500 response carries no SQL.
func TestHandler_InternalErrorsDoNotLeakDetails(t *testing.T) {
	for _, path := range []string{
		pushesPath,
		pushesPath + "/schedules",
		pushesPath + "/metrics/aggregate",
		pushesPath + "/1",
		pushesPath + "/schedules/1",
		pushesPath + "/metrics/campaign/1",
	} {
		t.Run(path, func(t *testing.T) {
			f := newTimerFixture(t)
			timerTestingT = t
			router := newPushTestRouter(f)
			for _, table := range []string{"delivery_attempts", "push_message_targets", "push_messages", "scheduled_push_targets", "scheduled_push_timers", "scheduled_pushes"} {
				if err := f.db.Exec("DROP TABLE " + table).Error; err != nil {
					t.Fatalf("drop %s: %v", table, err)
				}
			}
			assertGenericInternalError(t, doPushRequest(t, router, http.MethodGet, path, nil))
		})
	}
}

func TestWriteServiceError_UnknownErrorIsGeneric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, pushesPath, nil)
	h := &Handler{}
	h.writeServiceError(c, fmt.Errorf("insert push: %w", errors.New("Error 1062: Duplicate entry 'secret'")))
	assertGenericInternalError(t, w)
}

func TestWriteServiceError_MapsSentinels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrForbidden, http.StatusForbidden},
		{ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("%w: title and body are required", ErrInvalidPayload), http.StatusBadRequest},
		{fmt.Errorf("%w: bad cron", ErrInvalidSchedule), http.StatusBadRequest},
		{ErrDisabledFeature, http.StatusForbidden},
		{gorm.ErrRecordNotFound, http.StatusNotFound},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, pushesPath, nil)
			(&Handler{}).writeServiceError(c, tc.err)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body=%q)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// TestHandler_MissingRowsAreNotFound covers ids that do not exist
// or belong to another user: the repository answers
// gorm.ErrRecordNotFound, which must surface as 404, not 500.
func TestHandler_MissingRowsAreNotFound(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	router := newPushTestRouter(f)
	for _, tc := range []struct {
		method string
		path   string
		body   proto.Message
	}{
		{http.MethodGet, pushesPath + "/999", nil},
		{http.MethodGet, pushesPath + "/schedules/999", nil},
		{http.MethodPatch, pushesPath + "/schedules/999", &pb.UpdateScheduleRequest{IsActive: false}},
		{http.MethodDelete, pushesPath + "/schedules/999", nil},
		{http.MethodGet, pushesPath + "/metrics/campaign/999", nil},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := doPushRequest(t, router, tc.method, tc.path, tc.body)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body=%q)", w.Code, w.Body.String())
			}
		})
	}
}

func createImmediateViaHTTP(t *testing.T, f *timerFixture, router *gin.Engine) *pb.PushMessage {
	t.Helper()
	var created pb.PushMessage
	decodePushResponse(t, doPushRequest(t, router, http.MethodPost, pushesPath, &pb.CreateImmediatePushRequest{
		DomainIds: []uint32{uint32(f.domainID)},
		Payload:   adminPayload(),
	}), http.StatusCreated, &created)
	return &created
}

func TestHandler_CreateImmediate(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("http-1", "UTC")
	router := newPushTestRouter(f)

	created := createImmediateViaHTTP(t, f, router)
	if created.Id == 0 || created.Title != "t" || created.Source != string(SourceImmediate) {
		t.Fatalf("created = %v, want an immediate push titled t", created)
	}
	if len(created.DomainIds) != 1 || created.DomainIds[0] != uint32(f.domainID) {
		t.Fatalf("domain_ids = %v, want [%d]", created.DomainIds, f.domainID)
	}

	var stored PushMessage
	if err := f.db.First(&stored, created.Id).Error; err != nil {
		t.Fatalf("push not persisted: %v", err)
	}
}

func TestHandler_CreateImmediate_RejectsInvalidInput(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	router := newPushTestRouter(f)

	payload := adminPayload()
	payload.Title = ""
	w := doPushRequest(t, router, http.MethodPost, pushesPath, &pb.CreateImmediatePushRequest{
		DomainIds: []uint32{uint32(f.domainID)},
		Payload:   payload,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty title status = %d, want 400 (body=%q)", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, pushesPath, strings.NewReader("\xff\xff not protobuf"))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("garbage body status = %d, want 400 (body=%q)", w.Code, w.Body.String())
	}
}

func TestHandler_ListAndGetPushes(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("http-list", "UTC")
	router := newPushTestRouter(f)
	created := createImmediateViaHTTP(t, f, router)

	var page pb.PushMessagePage
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, pushesPath, nil), http.StatusOK, &page)
	if len(page.Data) != 1 || page.Data[0].Id != created.Id {
		t.Fatalf("list = %v, want the created push", page.Data)
	}
	if len(page.Data[0].DomainIds) != 1 || page.Data[0].DomainIds[0] != uint32(f.domainID) {
		t.Fatalf("listed domain_ids = %v, want [%d]", page.Data[0].DomainIds, f.domainID)
	}

	var got pb.PushMessage
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, fmt.Sprintf("%s/%d", pushesPath, created.Id), nil), http.StatusOK, &got)
	if got.Id != created.Id {
		t.Fatalf("get id = %d, want %d", got.Id, created.Id)
	}
}

func TestHandler_ScheduleLifecycle(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("http-sched", "UTC")
	router := newPushTestRouter(f)

	var created pb.ScheduledPush
	decodePushResponse(t, doPushRequest(t, router, http.MethodPost, pushesPath+"/schedules", &pb.CreateScheduleRequest{
		DomainIds:    []uint32{uint32(f.domainID)},
		Payload:      adminPayload(),
		ScheduleType: pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT,
		RunAt:        time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}), http.StatusCreated, &created)
	if created.Id == 0 || !created.IsActive {
		t.Fatalf("created = %v, want an active schedule", &created)
	}
	itemPath := fmt.Sprintf("%s/schedules/%d", pushesPath, created.Id)

	var page pb.ScheduledPushPage
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, pushesPath+"/schedules", nil), http.StatusOK, &page)
	if len(page.Data) != 1 || page.Data[0].Id != created.Id {
		t.Fatalf("list = %v, want the created schedule", page.Data)
	}

	var got pb.ScheduledPush
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, itemPath, nil), http.StatusOK, &got)
	if got.Id != created.Id || len(got.DomainIds) != 1 {
		t.Fatalf("get = %v, want schedule %d with its domain", &got, created.Id)
	}

	var paused pb.ScheduledPush
	decodePushResponse(t, doPushRequest(t, router, http.MethodPatch, itemPath, &pb.UpdateScheduleRequest{IsActive: false}), http.StatusOK, &paused)
	if paused.IsActive {
		t.Fatal("PATCH is_active=false left the schedule active")
	}

	var status pb.StatusMessage
	decodePushResponse(t, doPushRequest(t, router, http.MethodDelete, itemPath, nil), http.StatusOK, &status)
	if status.Message != "schedule deleted" {
		t.Fatalf("delete message = %q", status.Message)
	}
}

func TestHandler_CreateSchedule_RejectsInvalidSchedule(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("http-bad-sched", "UTC")
	router := newPushTestRouter(f)

	for name, req := range map[string]*pb.CreateScheduleRequest{
		"bad run_at": {DomainIds: []uint32{uint32(f.domainID)}, Payload: adminPayload(), ScheduleType: pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT, RunAt: "tomorrow"},
		"bad cron":   {DomainIds: []uint32{uint32(f.domainID)}, Payload: adminPayload(), ScheduleType: pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, CronExpr: "not a cron"},
	} {
		t.Run(name, func(t *testing.T) {
			if w := doPushRequest(t, router, http.MethodPost, pushesPath+"/schedules", req); w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%q)", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_Metrics(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("http-metrics", "UTC")
	router := newPushTestRouter(f)
	created := createImmediateViaHTTP(t, f, router)

	var aggregate pb.PushMetricsAggregate
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, pushesPath+"/metrics/aggregate", nil), http.StatusOK, &aggregate)
	if aggregate.MessagesSentTotal != 1 || aggregate.AudienceSize != 1 {
		t.Fatalf("aggregate = %v, want 1 sent to an audience of 1", &aggregate)
	}

	var campaign pb.PushMetricsByCampaign
	decodePushResponse(t, doPushRequest(t, router, http.MethodGet, fmt.Sprintf("%s/metrics/campaign/%d", pushesPath, created.Id), nil), http.StatusOK, &campaign)
	if campaign.PushId != created.Id || campaign.TargetsTotal != 1 {
		t.Fatalf("campaign = %v, want 1 target for push %d", &campaign, created.Id)
	}
}
