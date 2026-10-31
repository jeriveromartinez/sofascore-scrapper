//go:build integration

package devices

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func openDevicesMariaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
		envOr("DB_USER", "root"), os.Getenv("DB_PASSWORD"), envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "3306"), envOr("DB_NAME", "scores365"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql database: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	return db
}

// TestRegisterResolvesDomainAgainstMariaDB pins the LIKE clause against the
// production engine: the old ESCAPE '\' literal made MariaDB reject the query
// with error 1064, so Register could never resolve a device domain there.
// Only rows owned by this test are created and removed.
func TestRegisterResolvesDomainAgainstMariaDB(t *testing.T) {
	db := openDevicesMariaDB(t)
	if err := db.AutoMigrate(&users.User{}, &apk.ApkVersion{}, &domains.Domain{}, &Device{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	token := "itest-escape-token-" + suffix
	packageID := "itest-escape-pkg-" + suffix
	host := "itest-escape-" + suffix + ".example.test"

	user := users.User{Email: "itest-escape-" + suffix + "@example.test", Password: "not-a-real-hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&user) })

	apkRow := apk.ApkVersion{
		Version:       "itest-escape-" + suffix,
		FileName:      "itest-escape.apk",
		FilePath:      "itest-escape/apk",
		DownloadToken: "itest-escape-download-" + suffix,
		PackageName:   packageID,
		IPTVUrl:       "http://" + host,
	}
	if err := db.Create(&apkRow).Error; err != nil {
		t.Fatalf("seed apk: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&apkRow) })

	// A subdomain ensures the LIKE fallback runs: Register tries an exact
	// domain match first.
	domainRow := domains.Domain{Domain: "panel." + host, UserID: user.ID}
	if err := db.Create(&domainRow).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&domainRow) })

	device, err := NewRepository(db).Register(context.Background(), nil, 0, token, "android", "Itest Box", "1.0", packageID, "")
	if err != nil {
		t.Fatalf("Register against MariaDB: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Where("token = ?", token).Delete(&Device{}) })

	if device.DomainID == nil || *device.DomainID != domainRow.ID {
		t.Fatalf("device domain: want %d, got %v", domainRow.ID, device.DomainID)
	}
}
