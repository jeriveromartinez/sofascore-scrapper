package apk

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

func TestAPKAdminHandlersDoNotLeakInternalErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	if err := db.Migrator().DropTable(&ApkVersion{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	router := gin.New()
	NewAdminHandler(NewRepository(db)).RegisterRoutes(router.Group(""), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})

	body, err := proto.Marshal(&pb.DeviceUrl{Url: "http://example.com/list.m3u"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cases := []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, "/apk/versions", nil},
		{http.MethodGet, "/apk/versions/page", nil},
		{http.MethodPut, "/apk/1", body},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-protobuf")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d body = %s, want 500", w.Code, w.Body.String())
			}
			if b := w.Body.String(); strings.Contains(b, "apk_versions") || strings.Contains(b, "no such table") {
				t.Fatalf("500 body leaks the underlying error: %s", b)
			}
		})
	}
}
