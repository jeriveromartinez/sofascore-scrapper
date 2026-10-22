package seeder

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// seedDBCounter hands out the name of the in-memory database each call
// gets. A shared-cache database survives until its last connection closes,
// so the name has to be unique per call.
var seedDBCounter atomic.Int64

func newAdminSeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	// The name is unique per call so a database left behind by an earlier
	// run cannot be picked up again, and the pool is closed so the database
	// does not outlive the test.
	dsn := fmt.Sprintf("file:seed-%d?mode=memory&cache=shared", seedDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// TestNewAdminSeedDBUsesAFreshDatabasePerCall pins the hermeticity of the
// in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one seeded.
func TestNewAdminSeedDBUsesAFreshDatabasePerCall(t *testing.T) {
	first := newAdminSeedDB(t)
	if err := SeedDefaultAdmin(context.Background(), first); err != nil {
		t.Fatalf("seed first database: %v", err)
	}

	second := newAdminSeedDB(t)
	var count int64
	if err := second.Model(&users.User{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("the second database holds %d users of the first one", count)
	}
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

// TestSeedDefaultAdmin_ExistingAdminRowIsNotAnError reproduces the
// first-boot race deterministically: the users count is zero (here
// because the row is soft-deleted; on a multi-backend boot because a
// sibling inserted it after this process counted) but admin@local
// already holds the unique email. That used to fail with a duplicate
// key error and stop the backend.
func TestSeedDefaultAdmin_ExistingAdminRowIsNotAnError(t *testing.T) {
	t.Setenv(DefaultAdminPasswordEnvVar, "operator-chosen-password")
	db := newAdminSeedDB(t)
	existing := users.User{Email: DefaultAdminEmail, Password: "existing-hash", Role: users.RoleAdmin}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing admin: %v", err)
	}
	if err := db.Delete(&existing).Error; err != nil {
		t.Fatalf("soft-delete existing admin: %v", err)
	}
	DefaultAdminPassword = ""

	if err := SeedDefaultAdmin(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var rows int64
	if err := db.Unscoped().Model(&users.User{}).Where("email = ?", DefaultAdminEmail).Count(&rows).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("admin rows = %d, want 1", rows)
	}
	if DefaultAdminPassword != "" {
		t.Error("DefaultAdminPassword is set although this call stored no password")
	}
}
