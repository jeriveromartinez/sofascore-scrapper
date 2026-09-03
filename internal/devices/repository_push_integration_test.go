//go:build integration

package devices

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
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
// device/domain/user/apk push-notifications flow. The underlying
// *sql.DB is closed via t.Cleanup so the shared cache is released.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := sharedCacheDSN(t)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &apk.ApkVersion{}, &Device{}); err != nil {
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

// seedApkDomain inserts the ApkVersion + Domain rows that Register
// expects to find. The package_id is what the Flutter app sends;
// the IPTVUrl is what the backend derives the domain from.
func seedApkDomain(t *testing.T, db *gorm.DB, packageID, iptvURL, domainName string) (uint, uint) {
	t.Helper()
	apkRow := apk.ApkVersion{
		PackageName: packageID,
		IPTVUrl:     iptvURL,
	}
	if err := db.Create(&apkRow).Error; err != nil {
		t.Fatalf("seed apk: %v", err)
	}
	dom := domains.Domain{Domain: domainName}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return apkRow.ID, dom.ID
}

// TestRegisterDerivesDomainFromPackage verifies that Register sets
// Device.DomainID from the matching ApkVersion's IPTVUrl. The push
// audience filter relies on this column.
func TestRegisterDerivesDomainFromPackage(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	device, err := repo.Register(nil, "tok-1", "android", "Test Box", "1.0", "com.example.app", "America/Mexico_City")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.DomainID == nil {
		t.Fatal("DomainID not set on returned device")
	}

	var got Device
	if err := db.First(&got, device.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got.DomainID == nil {
		t.Fatal("persisted DomainID = nil")
	}
}

// TestRegisterPersistsTimezone verifies that a non-empty timezone
// is stored on the device row and re-loaded correctly.
func TestRegisterPersistsTimezone(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	device, err := repo.Register(nil, "tok-tz-1", "android", "Box", "1.0", "com.example.app", "America/Mexico_City")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.Timezone != "America/Mexico_City" {
		t.Fatalf("returned Timezone = %q, want %q", device.Timezone, "America/Mexico_City")
	}

	var got Device
	if err := db.First(&got, device.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got.Timezone != "America/Mexico_City" {
		t.Fatalf("persisted Timezone = %q, want %q", got.Timezone, "America/Mexico_City")
	}
}

// TestRegisterEmptyTimezoneStoresEmpty verifies that an empty
// timezone (Flutter app did not provide one) is stored as "" rather
// than "UTC", and that the scheduler can treat it as UTC later.
func TestRegisterEmptyTimezoneStoresEmpty(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	device, err := repo.Register(nil, "tok-tz-2", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.Timezone != "" {
		t.Fatalf("returned Timezone = %q, want \"\"", device.Timezone)
	}
}

// TestRegisterReusesRowAndKeepsTimezone verifies that when a device
// with the same token re-registers with an EMPTY timezone, the
// previously stored timezone is preserved (matches the pattern used
// for domain_id: the app's automatic re- register on launch must not
// wipe fields the operator or the device set once).
func TestRegisterReusesRowAndKeepsTimezone(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	first, err := repo.Register(nil, "tok-tz-3", "android", "Box", "1.0", "com.example.app", "America/Bogota")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first.Timezone != "America/Bogota" {
		t.Fatalf("first.Timezone = %q, want %q", first.Timezone, "America/Bogota")
	}

	second, err := repo.Register(nil, "tok-tz-3", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %d, want %d (same row)", second.ID, first.ID)
	}
	if second.Timezone != "America/Bogota" {
		t.Fatalf("second.Timezone = %q, want %q (preserved)", second.Timezone, "America/Bogota")
	}
}

// TestRegisterUpdatesTimezoneWhenProvided verifies the opposite
// direction: a re-register that passes a non-empty timezone DOES
// overwrite the previous value. Operators and the Flutter app can
// legitimately update the TZ when the user moves or changes
// locale.
func TestRegisterUpdatesTimezoneWhenProvided(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	if _, err := repo.Register(nil, "tok-tz-4", "android", "Box", "1.0", "com.example.app", "America/Bogota"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	second, err := repo.Register(nil, "tok-tz-4", "android", "Box", "1.0", "com.example.app", "Europe/Madrid")
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second.Timezone != "Europe/Madrid" {
		t.Fatalf("second.Timezone = %q, want %q (updated)", second.Timezone, "Europe/Madrid")
	}
}

// TestRegisterUpdatesLastSeen guards a regression: a fresh Register
// must always bump last_seen, otherwise the device looks offline.
func TestRegisterUpdatesLastSeen(t *testing.T) {
	db := newTestDB(t)
	seedApkDomain(t, db, "com.example.app", "iptv://example.com", "example.com")
	repo := NewRepository(db)

	before := time.Now().Unix()
	device, err := repo.Register(nil, "tok-5", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if device.LastSeen < before {
		t.Fatalf("LastSeen = %d, want >= %d", device.LastSeen, before)
	}
}