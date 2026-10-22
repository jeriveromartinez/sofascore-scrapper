package devices

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func newDevicesTestRouter(db *gorm.DB) *gin.Engine {
	repo := NewRepository(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAdminHandler(repo).RegisterRoutes(router.Group("/admin"), AdminHandlerDeps{AuthMiddleware: func(c *gin.Context) { c.Next() }})
	NewAppHandler(repo).RegisterRoutes(router.Group("/app"))
	return router
}

func sendDeviceRequest(t *testing.T, router *gin.Engine, method, url string, req *pb.DeviceRegisterRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(method, url, bytes.NewReader(body)))
	return w
}

func TestRegisterDeviceWithUnknownPackageOrDomainIsBadRequest(t *testing.T) {
	db := newDevicesTestDB(t)
	apkRow := apk.ApkVersion{Version: "1.0.0", FileName: "a.apk", FilePath: "/tmp/a.apk", PackageName: "com.nodomain.app", IPTVUrl: "https://unregistered.example", DownloadToken: "dl-1"}
	if err := db.Create(&apkRow).Error; err != nil {
		t.Fatalf("seed apk: %v", err)
	}
	router := newDevicesTestRouter(db)

	for _, packageID := range []string{"com.unknown.app", "com.nodomain.app"} {
		w := sendDeviceRequest(t, router, http.MethodPost, "/app/devices", &pb.DeviceRegisterRequest{Token: "tok", Name: "Box", PackageId: packageID})
		if w.Code != http.StatusBadRequest || w.Body.String() != `{"error":"invalid device registration"}` {
			t.Errorf("package %s = %d %s, want 400 invalid device registration", packageID, w.Code, w.Body.String())
		}
	}
}

func TestUpdateDeviceWithUnknownTokenIsNotFound(t *testing.T) {
	router := newDevicesTestRouter(newDevicesTestDB(t))

	w := sendDeviceRequest(t, router, http.MethodPut, "/admin/devices", &pb.DeviceRegisterRequest{Token: "missing", Name: "Box", PackageId: "com.example.app"})
	if w.Code != http.StatusNotFound || w.Body.String() != `{"error":"device not found"}` {
		t.Fatalf("unknown token = %d %s, want 404 device not found", w.Code, w.Body.String())
	}
}
