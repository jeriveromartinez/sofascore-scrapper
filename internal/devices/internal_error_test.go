package devices

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

const genericInternalErrorBody = `{"error":"internal server error"}`

func TestHandlersHideDatabaseErrorsBehindGenericMessage(t *testing.T) {
	db := newDevicesTestDB(t)
	repo := NewRepository(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAdminHandler(repo).RegisterRoutes(router.Group("/admin"), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})
	NewAppHandler(repo).RegisterRoutes(router.Group("/app"))

	for _, table := range []any{&Device{}, "apk_versions"} {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Fatalf("drop table: %v", err)
		}
	}

	device := &pb.DeviceRegisterRequest{Token: "tok", Name: "Box", PackageId: "com.example.app"}
	cases := []struct {
		method, url string
		body        proto.Message
	}{
		{http.MethodGet, "/admin/devices", nil},
		{http.MethodGet, "/admin/devices/page", nil},
		{http.MethodGet, "/admin/devices/all", nil},
		{http.MethodPut, "/admin/devices", device},
		{http.MethodPost, "/app/devices", device},
	}
	for _, tc := range cases {
		var body []byte
		if tc.body != nil {
			var err error
			if body, err = proto.Marshal(tc.body); err != nil {
				t.Fatalf("marshal: %v", err)
			}
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.url, bytes.NewReader(body)))
		if w.Code != http.StatusInternalServerError || w.Body.String() != genericInternalErrorBody {
			t.Errorf("%s %s = %d %s, want 500 %s", tc.method, tc.url, w.Code, w.Body.String(), genericInternalErrorBody)
		}
	}
}
