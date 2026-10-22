//go:build integration

package tournaments

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
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

func openMariaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
		envOr("DB_USER", "root"), os.Getenv("DB_PASSWORD"),
		envOr("DB_HOST", "localhost"), envOr("DB_PORT", "3306"), envOr("DB_NAME", "scores365"))
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("no database available: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &devices.Device{}, &Tournament{}, &DeviceTournament{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDeviceAssignments_ReassignAfterRemove_MariaDB(t *testing.T) {
	db := openMariaDB(t)
	const tournamentID uint = AdminTournamentIDCeiling - 1
	device := devices.Device{Token: "integration-reassign-device"}
	cleanup := func() {
		db.Unscoped().Where("tournament_id = ?", tournamentID).Delete(&DeviceTournament{})
		db.Unscoped().Where("token = ?", device.Token).Delete(&devices.Device{})
		db.Unscoped().Delete(&Tournament{}, tournamentID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if err := db.Create(&Tournament{Model: gorm.Model{ID: tournamentID}, Name: "Reassign Cup", Slug: "integration-reassign-cup"}).Error; err != nil {
		t.Fatalf("seed tournament: %v", err)
	}

	repo := NewDeviceAssignmentsRepository(db)
	if _, err := repo.Assign(device.ID, tournamentID); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	if err := repo.Remove(device.ID, tournamentID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := repo.Assign(device.ID, tournamentID); err != nil {
		t.Fatalf("re-assign after remove: %v", err)
	}
}
