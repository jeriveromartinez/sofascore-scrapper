//go:build integration

package users

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var usersTestCounter int64

// newNotifTestDB spins up a SQLite-shared DB with the user schema.
func newNotifTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&usersTestCounter, 1)
	dsn := fmt.Sprintf("file:test_users_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// newNoAuthRouter builds a router with the user routes mounted but
// no auth middleware. Authorization is the responsibility of
// whatever middleware is plugged in at wire-up; for the unit
// test we exercise the handler in isolation.
func newNoAuthRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	userRepo := NewRepository(db)
	h := &Handler{repo: userRepo}
	r.PUT("/api/admin/v1/users/:id/notifications", h.handleSetNotificationsEnabled)
	r.GET("/api/admin/v1/users/:id", h.handleGetUser)
	return r
}

func seedUserForHandlerTest(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	u := &User{Email: fmt.Sprintf("u-%d@x.com", time.Now().UnixNano()), Password: "x", Role: RoleUser}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return u.ID
}

// TestSetNotificationsEnabled_EnableStampsTimestamp verifies the
// happy path: PUT enabled=true stamps notifications_enabled_at
// (audit) and the response carries the new value.
func TestSetNotificationsEnabled_EnableStampsTimestamp(t *testing.T) {
	db := newNotifTestDB(t)
	uid := seedUserForHandlerTest(t, db)
	router := newNoAuthRouter(db)

	body, err := proto.Marshal(&pb.SetNotificationsEnabledRequest{Enabled: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/admin/v1/users/%d/notifications", uid),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got pb.User
	if err := proto.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !got.NotificationsEnabled {
		t.Errorf("NotificationsEnabled = false, want true")
	}
	if got.NotificationsEnabledAt == "" {
		t.Errorf("NotificationsEnabledAt empty, want RFC3339 stamp")
	}

	var row User
	if err := db.First(&row, uid).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !row.NotificationsEnabled {
		t.Error("persisted NotificationsEnabled = false")
	}
	if row.NotificationsEnabledAt == nil {
		t.Error("persisted NotificationsEnabledAt = nil")
	}
}

// TestSetNotificationsEnabled_DisableLeavesTimestamp covers the
// disable path: the flag flips to false, but the original
// activation timestamp is preserved (audit only).
func TestSetNotificationsEnabled_DisableLeavesTimestamp(t *testing.T) {
	db := newNotifTestDB(t)
	uid := seedUserForHandlerTest(t, db)
	router := newNoAuthRouter(db)

	// Enable first.
	enableBody, _ := proto.Marshal(&pb.SetNotificationsEnabledRequest{Enabled: true})
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/admin/v1/users/%d/notifications", uid),
		bytes.NewReader(enableBody))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var enabled pb.User
	if err := proto.Unmarshal(rec.Body.Bytes(), &enabled); err != nil {
		t.Fatalf("unmarshal enable: %v", err)
	}
	if enabled.NotificationsEnabledAt == "" {
		t.Fatal("NotificationsEnabledAt empty after enable")
	}

	// Disable.
	disableBody, _ := proto.Marshal(&pb.SetNotificationsEnabledRequest{Enabled: false})
	req2 := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/admin/v1/users/%d/notifications", uid),
		bytes.NewReader(disableBody))
	req2.Header.Set("Content-Type", "application/x-protobuf")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("disable: status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	var got pb.User
	if err := proto.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NotificationsEnabled {
		t.Error("NotificationsEnabled = true, want false")
	}
	if got.NotificationsEnabledAt == "" {
		t.Error("NotificationsEnabledAt lost on disable; want preserved")
	}
	if got.NotificationsEnabledAt != enabled.NotificationsEnabledAt {
		t.Errorf("NotificationsEnabledAt = %q, want unchanged %q", got.NotificationsEnabledAt, enabled.NotificationsEnabledAt)
	}
}

// TestSetNotificationsEnabled_UnknownUserIs404 covers the not-found
// path: PUT to a user id that does not exist returns 404, not 500.
func TestSetNotificationsEnabled_UnknownUserIs404(t *testing.T) {
	db := newNotifTestDB(t)
	router := newNoAuthRouter(db)

	body, _ := proto.Marshal(&pb.SetNotificationsEnabledRequest{Enabled: true})
	req := httptest.NewRequest(http.MethodPut,
		"/api/admin/v1/users/9999/notifications",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
