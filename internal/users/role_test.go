package users

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func newUsersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func TestSetRoleValidatesAndUpdates(t *testing.T) {
	db := newUsersTestDB(t)
	repo := NewRepository(db)
	u, err := repo.Create("promote@test.local", "password123")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := repo.SetRole(context.Background(), u.ID, "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}

	updated, err := repo.SetRole(context.Background(), u.ID, RoleAdmin)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !updated.IsAdmin() {
		t.Fatalf("expected admin, got role=%q", updated.Role)
	}

	count, err := repo.CountAdmins()
	if err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin count=%d, want 1", count)
	}
}

func newRoleTestRouter(repo *Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(repo)
	h.RegisterUserRoutes(r.Group("/api/web/v1"), HandlerDeps{AuthMiddleware: func(c *gin.Context) {}})
	return r
}

func setRoleRequest(t *testing.T, router *gin.Engine, id uint, role string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := proto.Marshal(&pb.SetUserRoleRequest{Role: role})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := "/api/web/v1/users/" + strconv.FormatUint(uint64(id), 10) + "/role"
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSetRoleEndpointCannotDemoteLastAdmin(t *testing.T) {
	db := newUsersTestDB(t)
	admin := User{Email: "solo-admin@test.local", Password: "x", Role: RoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	repo := NewRepository(db)
	router := newRoleTestRouter(repo)

	if w := setRoleRequest(t, router, admin.ID, RoleUser); w.Code != http.StatusConflict {
		t.Fatalf("demote-last-admin status=%d, want %d (body=%q)", w.Code, http.StatusConflict, w.Body.String())
	}

	// With a second admin present, demotion is allowed.
	second := User{Email: "second-admin@test.local", Password: "x", Role: RoleAdmin}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("seed second admin: %v", err)
	}
	if w := setRoleRequest(t, router, admin.ID, RoleUser); w.Code != http.StatusOK {
		t.Fatalf("demote-with-spare status=%d, want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestSetRoleConcurrentDemotionsRejectBothLastAdmins proves the
// SetRole FOR UPDATE lock actually closes the last-admin race.
// Two admins are seeded; two goroutines demote them concurrently.
// Without the lock, both could observe admins=2 under REPEATABLE
// READ and both succeed, leaving zero admins. With the lock, exactly
// one wins and the other observes the now-demoted state and is
// rejected with ErrLastAdmin.
//
// The test uses a per-test on-disk SQLite file rather than ":memory:"
// because ":memory:" databases are per-connection — gorm's pool
// gives each goroutine a fresh connection, and the second connection
// would not see the first's schema. A temp file is shared across
// all connections to the same path.
func TestSetRoleConcurrentDemotionsRejectBothLastAdmins(t *testing.T) {
	db := newUsersTestDBShared(t)
	repo := NewRepository(db)
	first := User{Email: "race-1@test.local", Password: "x", Role: RoleAdmin}
	second := User{Email: "race-2@test.local", Password: "x", Role: RoleAdmin}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("seed second: %v", err)
	}

	results := make(chan error, 2)
	go func() { _, err := repo.SetRole(context.Background(), first.ID, RoleUser); results <- err }()
	go func() { _, err := repo.SetRole(context.Background(), second.ID, RoleUser); results <- err }()

	got := 0
	var lastErr error
	for i := 0; i < 2; i++ {
		err := <-results
		got++
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("demote %d: want ErrLastAdmin, got %v", got, err)
		}
		lastErr = err
	}

	// Exactly one of the two should be rejected (the one that loses
	// the lock race). With a sqlite test driver the racing order is
	// not deterministic, so we just verify at least one got
	// ErrLastAdmin — i.e. we never demoted both.
	if lastErr == nil {
		t.Fatalf("both concurrent demotions succeeded; last-admin race is not closed")
	}

	var admins int64
	if err := db.Model(&User{}).Where("role = ?", RoleAdmin).Count(&admins).Error; err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if admins != 1 {
		t.Fatalf("after concurrent demotions: want 1 admin remaining, got %d", admins)
	}
}

func newUsersTestDBShared(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "race.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open race database: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate race database: %v", err)
	}
	// Close the connection pool when the test finishes so the
	// TempDir cleanup can unlink the on-disk SQLite files; without
	// this, Windows fails TempDir RemoveAll with "file in use".
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		_ = sqlDB.Close()
	})
	return db
}
