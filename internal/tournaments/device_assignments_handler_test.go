package tournaments

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
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
