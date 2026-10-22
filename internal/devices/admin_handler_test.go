package devices

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func TestGetAllDevicesLegacyRouteIsLimitedTo1000(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &Device{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	devices := make([]Device, 1001)
	for i := range devices {
		devices[i].Token = fmt.Sprintf("device-%04d", i)
	}
	if err := db.CreateInBatches(devices, 200).Error; err != nil {
		t.Fatalf("seed devices: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAdminHandler(NewRepository(db)).RegisterRoutes(router.Group(""), AdminHandlerDeps{
		AuthMiddleware: func(c *gin.Context) { c.Next() },
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/devices/all", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, recorder.Code)
	}
	response := &pb.DeviceList{}
	if err := proto.Unmarshal(recorder.Body.Bytes(), response); err != nil {
		t.Fatalf("decode devices response: %v", err)
	}
	if len(response.Data) != 1000 {
		t.Fatalf("devices: want fixed maximum 1000, got %d", len(response.Data))
	}
	if response.Data[0].Id != 1001 || response.Data[len(response.Data)-1].Id != 2 {
		t.Fatalf("device IDs: want newest range 1001..2, got %d..%d", response.Data[0].Id, response.Data[len(response.Data)-1].Id)
	}
}

func TestGetDevicesLegacyRoutePagesNewestFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &Device{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := db.Create(&Device{Token: fmt.Sprintf("device-%d", i)}).Error; err != nil {
			t.Fatalf("seed device %d: %v", i, err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAdminHandler(NewRepository(db)).RegisterRoutes(router.Group(""), AdminHandlerDeps{
		AuthMiddleware: func(c *gin.Context) { c.Next() },
	})

	wantPages := map[int][]uint32{1: {5, 4, 3}, 2: {2, 1}}
	for page, wantIDs := range wantPages {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/devices?page=%d&limit=3", page), nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("page %d status: want %d, got %d", page, http.StatusOK, recorder.Code)
		}
		response := &pb.DeviceList{}
		if err := proto.Unmarshal(recorder.Body.Bytes(), response); err != nil {
			t.Fatalf("decode page %d: %v", page, err)
		}
		if response.Total != 5 || response.TotalPages != 2 {
			t.Fatalf("page %d totals: want 5/2, got %d/%d", page, response.Total, response.TotalPages)
		}
		gotIDs := make([]uint32, 0, len(response.Data))
		for _, d := range response.Data {
			gotIDs = append(gotIDs, d.Id)
		}
		if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
			t.Fatalf("page %d ids: want %v, got %v", page, wantIDs, gotIDs)
		}
	}
}
