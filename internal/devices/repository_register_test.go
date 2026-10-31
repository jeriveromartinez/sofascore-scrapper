package devices

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"gorm.io/gorm"
)

func seedApkVersion(t *testing.T, db *gorm.DB, packageID, version, iptvURL string, active bool) {
	t.Helper()
	major, minor, patch, err := apk.ParseSemverComponents(version)
	if err != nil {
		t.Fatalf("parse semver %q: %v", version, err)
	}
	row := apk.ApkVersion{
		Version: version, FileName: version + ".apk", FilePath: "/tmp/" + version + ".apk",
		PackageName: packageID, IPTVUrl: iptvURL, DownloadToken: "dl-" + version,
		VersionMajor: major, VersionMinor: minor, VersionPatch: patch, IsActive: active,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed apk %s: %v", version, err)
	}
	if !active {
		// gorm omits zero-valued fields that carry a database default, so
		// the INSERT above still stores is_active = true.
		if err := db.Model(&row).UpdateColumn("is_active", false).Error; err != nil {
			t.Fatalf("deactivate apk %s: %v", version, err)
		}
	}
}

func seedDomain(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	row := domains.Domain{Domain: name}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed domain %s: %v", name, err)
	}
	return row.ID
}

func assertDomainID(t *testing.T, device *Device, want uint) {
	t.Helper()
	if device.DomainID == nil {
		t.Fatalf("DomainID = nil, want %d", want)
	}
	if *device.DomainID != want {
		t.Fatalf("DomainID = %d, want %d", *device.DomainID, want)
	}
}

func TestRegisterUsesNewestActiveApkVersion(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "old.example.com")
	seedDomain(t, db, "inactive.example.com")
	want := seedDomain(t, db, "newest.example.com")

	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://old.example.com", true)
	seedApkVersion(t, db, "com.example.app", "2.0.0", "http://newest.example.com", true)
	seedApkVersion(t, db, "com.example.app", "3.0.0", "http://inactive.example.com", false)

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-newest", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterDerivesDomainFromHostWithPortAndPath(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "5.mdtv.me")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://5.mdtv.me:8080/", true)

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-port", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterDoesNotBindSiblingDomainWithSameSuffix(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "panel5.mdtv.me")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://5.mdtv.me", true)

	_, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-sibling", "android", "Box", "1.0", "com.example.app", "")
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("register with sibling-only domain: want gorm.ErrInvalidData, got %v", err)
	}
}

func TestRegisterMatchesSubdomainOfApkHost(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "panel.5.mdtv.me")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://5.mdtv.me", true)

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-subdomain", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterPrefersExactDomainOverSubdomain(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "5.mdtv.me")
	seedDomain(t, db, "panel.5.mdtv.me")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://5.mdtv.me", true)

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-exact-domain", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterUsesExplicitDomainID(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "derived.example.com")
	want := seedDomain(t, db, "explicit.example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://derived.example.com", true)

	device, err := NewRepository(db).Register(context.Background(), nil, want, "tok-explicit", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterRejectsUnknownDomainID(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "derived.example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://derived.example.com", true)

	_, err := NewRepository(db).Register(context.Background(), nil, 4242, "tok-bad-domain", "android", "Box", "1.0", "com.example.app", "")
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("register with unknown domain_id: want gorm.ErrInvalidData, got %v", err)
	}
}

func TestRegisterRejectsSoftDeletedDomainID(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "derived.example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://derived.example.com", true)

	deleted := domains.Domain{Domain: "deleted.example.com"}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("soft delete domain: %v", err)
	}

	_, err := NewRepository(db).Register(context.Background(), nil, deleted.ID, "tok-deleted-domain", "android", "Box", "1.0", "com.example.app", "")
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("register with soft-deleted domain_id: want gorm.ErrInvalidData, got %v", err)
	}
}

func TestRegisterAcceptsEmptyName(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://example.com", true)

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-noname", "android", "", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register with empty name: %v", err)
	}
	assertDomainID(t, device, want)
}

func TestRegisterRejectsTokenLongerThanColumn(t *testing.T) {
	db := newDevicesTestDB(t)
	seedDomain(t, db, "example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://example.com", true)

	_, err := NewRepository(db).Register(context.Background(), nil, 0, strings.Repeat("t", 192), "android", "Box", "1.0", "com.example.app", "")
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("register with 192-byte token: want gorm.ErrInvalidData, got %v", err)
	}

	if _, err := NewRepository(db).Register(context.Background(), nil, 0, strings.Repeat("t", 191), "android", "Box", "1.0", "com.example.app", ""); err != nil {
		t.Fatalf("register with 191-byte token: %v", err)
	}
}

func TestRegisterRestoresSoftDeletedDevice(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://example.com", true)

	existing := Device{Token: "tok-deleted", Name: "Old"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if err := db.Delete(&existing).Error; err != nil {
		t.Fatalf("soft delete device: %v", err)
	}

	device, err := NewRepository(db).Register(context.Background(), nil, 0, "tok-deleted", "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register soft-deleted token: %v", err)
	}
	if device.ID != existing.ID {
		t.Fatalf("ID = %d, want %d (restored row)", device.ID, existing.ID)
	}
	assertDomainID(t, device, want)

	var restored Device
	if err := db.Where("token = ?", "tok-deleted").First(&restored).Error; err != nil {
		t.Fatalf("restored row is not visible to the default scope: %v", err)
	}
	if restored.DeletedAt.Valid {
		t.Fatal("restored row still has deleted_at set")
	}
}

func TestRegisterRetriesWhenConcurrentInsertWins(t *testing.T) {
	db := newDevicesTestDB(t)
	want := seedDomain(t, db, "example.com")
	seedApkVersion(t, db, "com.example.app", "1.0.0", "http://example.com", true)

	const token = "tok-race"
	var injected bool
	if err := db.Callback().Query().After("gorm:query").Register("test:inject_concurrent_device", func(tx *gorm.DB) {
		if injected || tx.Statement.Table != "devices" {
			return
		}
		if _, ok := tx.Statement.Dest.(*Device); !ok {
			return
		}
		injected = true
		winner := Device{Token: token, Name: "winner"}
		if err := db.Create(&winner).Error; err != nil {
			t.Fatalf("inject concurrent device: %v", err)
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	device, err := NewRepository(db).Register(context.Background(), nil, 0, token, "android", "Box", "1.0", "com.example.app", "")
	if err != nil {
		t.Fatalf("register after losing insert race: %v", err)
	}
	if device.Name != "Box" {
		t.Fatalf("Name = %q, want %q (retry must update the winning row)", device.Name, "Box")
	}
	assertDomainID(t, device, want)

	var count int64
	if err := db.Model(&Device{}).Where("token = ?", token).Count(&count).Error; err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if count != 1 {
		t.Fatalf("token rows = %d, want 1", count)
	}
}

func TestEscapeLikeUsesExclamationEscape(t *testing.T) {
	if got := escapeLike("a_b%c!d"); got != "a!_b!%c!!d" {
		t.Fatalf("escapeLike = %q, want %q", got, "a!_b!%c!!d")
	}
}

func TestHostFromIPTVURL(t *testing.T) {
	cases := map[string]string{
		"http://5.mdtv.me:8080/": "5.mdtv.me",
		"https://5.mdtv.me/path": "5.mdtv.me",
		"5.mdtv.me":              "5.mdtv.me",
		"iptv://example.com":     "example.com",
		"":                       "",
		"http://":                "",
	}
	for raw, want := range cases {
		if got := hostFromIPTVURL(raw); got != want {
			t.Errorf("hostFromIPTVURL(%q) = %q, want %q", raw, got, want)
		}
	}
}
