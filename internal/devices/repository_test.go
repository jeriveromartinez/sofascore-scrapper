package devices

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

func newDevicesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &apk.ApkVersion{}, &Device{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestGetDevicesHonoursCancelledContext(t *testing.T) {
	repo := NewRepository(newDevicesTestDB(t))

	if _, _, err := repo.GetDevices(cancelledContext(), 1, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetDevices with cancelled ctx: want context.Canceled, got %v", err)
	}
}

func TestRegisterHonoursCancelledContext(t *testing.T) {
	repo := NewRepository(newDevicesTestDB(t))

	if _, err := repo.Register(cancelledContext(), nil, "tok", "android", "Box", "1.0", "com.example.app", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("Register with cancelled ctx: want context.Canceled, got %v", err)
	}
}

func TestRegisterRejectsUnknownPackageAsInvalidData(t *testing.T) {
	repo := NewRepository(newDevicesTestDB(t))

	if _, err := repo.Register(context.Background(), nil, "tok", "android", "Box", "1.0", "com.unknown.app", ""); !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("Register with unknown package: want gorm.ErrInvalidData, got %v", err)
	}
}

func TestGetAllHonoursCancelledContext(t *testing.T) {
	repo := NewRepository(newDevicesTestDB(t))

	if _, err := repo.GetAll(cancelledContext()); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetAll with cancelled ctx: want context.Canceled, got %v", err)
	}
}

func TestFindByTokenAndUpdateHonourCancelledContext(t *testing.T) {
	db := newDevicesTestDB(t)
	if err := db.Create(&Device{Token: "tok"}).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	repo := NewRepository(db)
	ctx := cancelledContext()

	if _, err := repo.FindByToken(ctx, "tok"); !errors.Is(err, context.Canceled) {
		t.Errorf("FindByToken with cancelled ctx: want context.Canceled, got %v", err)
	}
	if _, err := repo.Update(ctx, "tok", "android", "Box", "com.example.app", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("Update with cancelled ctx: want context.Canceled, got %v", err)
	}
}

func TestAppMiddlewareUsesRequestContext(t *testing.T) {
	db := newDevicesTestDB(t)
	if err := db.Create(&Device{Token: "tok"}).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ping", AppMiddleware(db), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	live := httptest.NewRequest(http.MethodGet, "/ping", nil)
	live.Header.Set("APP-XIPTV", "tok")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, live)
	if w.Code != http.StatusNoContent {
		t.Fatalf("known token with live ctx = %d, want 204", w.Code)
	}

	cancelled := live.Clone(cancelledContext())
	w = httptest.NewRecorder()
	router.ServeHTTP(w, cancelled)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("known token with cancelled request ctx = %d, want 401 (lookup must run on the request context)", w.Code)
	}
}
