package users

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
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

	if _, err := repo.SetRole(u.ID, "superuser"); err != ErrInvalidRole {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}

	updated, err := repo.SetRole(u.ID, RoleAdmin)
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
