package devices

import (
	"context"
	"errors"
	"testing"

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
