package seeder

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func newAdminSeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:seed_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seededAdmin(t *testing.T, db *gorm.DB) users.User {
	t.Helper()
	var admin users.User
	if err := db.Where("email = ?", DefaultAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("default admin not created: %v", err)
	}
	return admin
}

// TestSeedDefaultAdmin_UsesPasswordFromEnv covers the configured
// path: the password of the seeded account is the one from the
// environment variable.
func TestSeedDefaultAdmin_UsesPasswordFromEnv(t *testing.T) {
	t.Setenv(DefaultAdminPasswordEnvVar, "operator-chosen-password")
	db := newAdminSeedDB(t)

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin := seededAdmin(t, db)
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("operator-chosen-password")); err != nil {
		t.Fatalf("stored hash does not match the configured password: %v", err)
	}
	if admin.Role != users.RoleAdmin {
		t.Errorf("role = %q, want %q", admin.Role, users.RoleAdmin)
	}
}

// TestSeedDefaultAdmin_GeneratesRandomPasswordWithoutEnv covers the
// fallback: without the environment variable the seeded password is
// random, not a known constant.
func TestSeedDefaultAdmin_GeneratesRandomPasswordWithoutEnv(t *testing.T) {
	t.Setenv(DefaultAdminPasswordEnvVar, "")
	db := newAdminSeedDB(t)

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin := seededAdmin(t, db)
	if DefaultAdminPassword == "" {
		t.Fatal("no password was resolved")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(DefaultAdminPassword)); err != nil {
		t.Fatalf("stored hash does not match the generated password: %v", err)
	}
}

// TestSeedDefaultAdmin_NeverUsesHardcodedPassword is the regression
// guard for the hardcoded default credential: no environment
// variable, no known password.
func TestSeedDefaultAdmin_NeverUsesHardcodedPassword(t *testing.T) {
	t.Setenv(DefaultAdminPasswordEnvVar, "")
	db := newAdminSeedDB(t)

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin := seededAdmin(t, db)
	for _, known := range []string{"admin1234", "admin", "password", "changeme"} {
		if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(known)); err == nil {
			t.Fatalf("the seeded admin still accepts a well-known password")
		}
	}
}
