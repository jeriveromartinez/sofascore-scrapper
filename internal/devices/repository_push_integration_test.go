//go:build integration

package devices

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// sharedCacheDSN builds a unique shared-cache SQLite DSN. Using the
// shared cache avoids the Windows file-handle cleanup race that hits
// on-disk temp files when GORM keeps the connection open past the
// test's TempDir RemoveAll.
var sharedCacheCounter int64

func sharedCacheDSN(t *testing.T) string {
	t.Helper()
	id := atomic.AddInt64(&sharedCacheCounter, 1)
	return fmt.Sprintf("file:test_%s_%d?mode=memory&cache=shared", t.Name(), id)
}

// newTestDB returns a GORM DB backed by an in-memory SQLite (shared
// cache, unique DSN per test) with the minimal schema needed for the
// device/domain/user push-notifications flow. The underlying *sql.DB
// is closed via t.Cleanup so the shared cache is released.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := sharedCacheDSN(t)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &Device{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil && err != sql.ErrConnDone {
			t.Logf("close sql.DB: %v", err)
		}
	})
	return db
}

// TestRegisterSetsDomainID verifies that Register persists the
// domainID when one is provided. This is the contract that drives the
// push audience filter (pushes to domain X only reach devices whose
// domain_id == X).
func TestRegisterSetsDomainID(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	domainID := uint(7)
	device, err := repo.Register(nil, &domainID, "tok-1", "android", "Test Box", "1.0")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.DomainID == nil {
		t.Fatal("DomainID not set on returned device")
	}
	if *device.DomainID != domainID {
		t.Fatalf("DomainID = %d, want %d", *device.DomainID, domainID)
	}
	// Re-read to confirm the value survived the round-trip.
	var got Device
	if err := db.First(&got, device.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got.DomainID == nil || *got.DomainID != domainID {
		t.Fatalf("persisted DomainID = %v, want %d", got.DomainID, domainID)
	}
}

// TestRegisterLeavesDomainIDNilWhenAbsent verifies that omitting
// domainID leaves the column NULL, so existing devices that never
// picked a domain stay NULL and are excluded from push delivery.
func TestRegisterLeavesDomainIDNilWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	device, err := repo.Register(nil, nil, "tok-2", "android", "Test Box", "1.0")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.DomainID != nil {
		t.Fatalf("DomainID = %v, want nil", *device.DomainID)
	}
}

// TestRegisterReusesRowAndKeepsDomainID verifies that when a device
// with the same token re-registers, the domainID is NOT silently
// cleared if the caller passes nil. The existing row's domain_id must
// be preserved; this avoids wiping a previously-set domain on every
// app restart.
func TestRegisterReusesRowAndKeepsDomainID(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	domainID := uint(11)
	first, err := repo.Register(nil, &domainID, "tok-3", "android", "Box", "1.0")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first.DomainID == nil || *first.DomainID != domainID {
		t.Fatalf("first.DomainID = %v, want %d", first.DomainID, domainID)
	}

	second, err := repo.Register(nil, nil, "tok-3", "android", "Box", "1.0")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %d, want %d (same row)", second.ID, first.ID)
	}
	if second.DomainID == nil || *second.DomainID != domainID {
		t.Fatalf("second.DomainID = %v, want %d (preserved)", second.DomainID, domainID)
	}
}

// TestRegisterUpdatesLastSeen guards a regression: a fresh Register
// must always bump last_seen, otherwise the device looks offline.
func TestRegisterUpdatesLastSeen(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	before := time.Now().Unix()
	device, err := repo.Register(nil, nil, "tok-4", "android", "Box", "1.0")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.LastSeen < before {
		t.Fatalf("LastSeen = %d, want >= %d", device.LastSeen, before)
	}
}
