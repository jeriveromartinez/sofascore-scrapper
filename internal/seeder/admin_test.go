//go:build integration

package seeder_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/seeder"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func dsnFromEnv() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsnFromEnv()), &gorm.Config{})
	if err != nil {
		t.Skipf("mariadb not reachable, skipping integration test: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("automigrate user: %v", err)
	}
	return db
}

func TestSeedDefaultAdmin_EmptyDBCreatesAdmin(t *testing.T) {
	db := newTestDB(t)
	// Ensure empty
	if err := db.Exec("DELETE FROM users").Error; err != nil {
		t.Fatalf("truncate users: %v", err)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var got users.User
	if err := db.Where("email = ?", seeder.DefaultAdminEmail).First(&got).Error; err != nil {
		t.Fatalf("admin not found: %v", err)
	}
	if got.Role != users.RoleAdmin {
		t.Errorf("role = %q, want %q", got.Role, users.RoleAdmin)
	}
}

func TestSeedDefaultAdmin_NonEmptyDBIsNoop(t *testing.T) {
	db := newTestDB(t)
	if err := db.Exec("DELETE FROM users").Error; err != nil {
		t.Fatalf("truncate users: %v", err)
	}
	pre := users.User{Email: "pre@test.local", Password: "x", Role: users.RoleUser}
	if err := db.Create(&pre).Error; err != nil {
		t.Fatalf("pre-insert: %v", err)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	if err := db.Model(&users.User{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (no extra admin)", count)
	}
}

func TestSeedDefaultAdmin_HashedPasswordMatches(t *testing.T) {
	db := newTestDB(t)
	if err := db.Exec("DELETE FROM users").Error; err != nil {
		t.Fatalf("truncate users: %v", err)
	}
	if err := seeder.SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var got users.User
	if err := db.Where("email = ?", seeder.DefaultAdminEmail).First(&got).Error; err != nil {
		t.Fatalf("admin not found: %v", err)
	}
	if got.Password == seeder.DefaultAdminPassword {
		t.Fatalf("password stored as plaintext, expected bcrypt hash")
	}
	if !looksLikeBcrypt(got.Password) {
		t.Errorf("password does not look like a bcrypt hash: %q", got.Password)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(seeder.DefaultAdminPassword)); err != nil {
		t.Errorf("bcrypt compare failed (hash should match plaintext): %v", err)
	}
}

func looksLikeBcrypt(s string) bool {
	re := regexp.MustCompile(`^\$2[abxy]\$\d{2}\$.{53}$`)
	return re.MatchString(s)
}
