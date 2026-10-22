package users

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const genericInternalErrorBody = `{"error":"internal server error"}`

func newUsersTestRouter(db *gorm.DB, callerID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(NewRepository(db)).RegisterUserRoutes(r.Group("/"), HandlerDeps{AuthMiddleware: func(c *gin.Context) {
		c.Set("userID", callerID)
		c.Next()
	}})
	return r
}

func sendProto(t *testing.T, r *gin.Engine, method, url string, msg proto.Message) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if msg != nil {
		var err error
		if body, err = proto.Marshal(msg); err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandlersHideDatabaseErrorsBehindGenericMessage(t *testing.T) {
	db := newUsersTestDB(t)
	r := newUsersTestRouter(db, 1)
	if err := db.Migrator().DropTable(&User{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	enable := &pb.SetNotificationsEnabledRequest{Enabled: true}
	cases := []struct {
		method, url string
		body        proto.Message
	}{
		{http.MethodGet, "/users", nil},
		{http.MethodGet, "/users/page", nil},
		{http.MethodDelete, "/users/1", nil},
		{http.MethodPut, "/users/1/notifications", enable},
		{http.MethodPut, "/users/2/notifications", enable},
	}
	for _, tc := range cases {
		w := sendProto(t, r, tc.method, tc.url, tc.body)
		if w.Code != http.StatusInternalServerError || w.Body.String() != genericInternalErrorBody {
			t.Errorf("%s %s = %d %s, want 500 %s", tc.method, tc.url, w.Code, w.Body.String(), genericInternalErrorBody)
		}
	}
}
