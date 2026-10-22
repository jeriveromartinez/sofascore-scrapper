package push

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
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
