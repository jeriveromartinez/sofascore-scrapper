//go:build integration

package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// newAuthDB returns a GORM DB with users, domains, and devices
// auto-migrated. Each test gets a unique shared-cache DSN so the
// tests are independent.
func newAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&authDBCounter, 1)
	dsn := fmt.Sprintf("file:test_auth_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(openDriver(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &devices.Device{}); err != nil {
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

var authDBCounter int64

// seedUserAndDomain inserts a user and a domain that the device can
// reference. Returns the user_id and domain_id for use in tests.
func seedUserAndDomain(t *testing.T, db *gorm.DB) (uint, uint) {
	t.Helper()
	u := &users.User{Email: "u@x.com", Password: "x", Role: users.RoleUser}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: "client.iptv.example", UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return u.ID, d.ID
}

// TestAuthenticator_ValidTokenReturnsDevice covers the happy path:
// the token maps to a real device row.
func TestAuthenticator_ValidTokenReturnsDevice(t *testing.T) {
	db := newAuthDB(t)
	uid, did := seedUserAndDomain(t, db)
	didP := did
	dev := &devices.Device{UserID: &uid, DomainID: &didP, Token: "tok-1", Platform: "android", Name: "Box", Version: "1.0"}
	if err := db.Create(dev).Error; err != nil {
		t.Fatalf("create device: %v", err)
	}

	a := NewAuthenticator(db)
	got, err := a.AuthenticateToken(context.Background(), "tok-1")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != dev.ID {
		t.Errorf("ID = %d, want %d", got.ID, dev.ID)
	}
	if got.DomainID == nil || *got.DomainID != did {
		t.Errorf("DomainID = %v, want %d", got.DomainID, did)
	}
}

// TestAuthenticator_UnknownTokenIsUnauthorized pins the rejection
// contract: the WS upgrade handler closes the socket with 4401 when
// AuthenticateToken returns ErrInvalidToken.
func TestAuthenticator_UnknownTokenIsUnauthorized(t *testing.T) {
	db := newAuthDB(t)
	a := NewAuthenticator(db)
	_, err := a.AuthenticateToken(context.Background(), "never-issued")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

// TestAuthenticator_DeviceWithoutDomainIsAllowedAtUpgrade covers
// the case where a device was registered before the push feature
// shipped (domain_id is NULL). The device is still allowed to
// connect; it just won't receive pushes until an admin links it to
// a domain.
func TestAuthenticator_DeviceWithoutDomainIsAllowedAtUpgrade(t *testing.T) {
	db := newAuthDB(t)
	uid, _ := seedUserAndDomain(t, db)
	dev := &devices.Device{UserID: &uid, Token: "tok-2", Platform: "android", Name: "Box", Version: "1.0"}
	if err := db.Create(dev).Error; err != nil {
		t.Fatalf("create device: %v", err)
	}
	a := NewAuthenticator(db)
	got, err := a.AuthenticateToken(context.Background(), "tok-2")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.DomainID != nil {
		t.Errorf("DomainID = %v, want nil", *got.DomainID)
	}
}
