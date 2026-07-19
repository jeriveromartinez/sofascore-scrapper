package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

func newRBACTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func TestRequireAdmin(t *testing.T) {
	db := newRBACTestDB(t)
	admin := users.User{Email: "admin@test.local", Password: "x", Role: users.RoleAdmin}
	regular := users.User{Email: "user@test.local", Password: "x", Role: users.RoleUser}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&regular).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	repo := users.NewRepository(db)

	gin.SetMode(gin.TestMode)
	newRouter := func(uid uint, setUser bool) *gin.Engine {
		r := gin.New()
		r.GET("/admin",
			func(c *gin.Context) {
				if setUser {
					c.Set(userIDKey, uid)
				}
			},
			RequireAdmin(repo),
			func(c *gin.Context) { c.String(http.StatusOK, "ok") },
		)
		return r
	}

	cases := []struct {
		name    string
		uid     uint
		setUser bool
		want    int
	}{
		{"admin is allowed", admin.ID, true, http.StatusOK},
		{"regular user is forbidden", regular.ID, true, http.StatusForbidden},
		{"unknown user is unauthorized", 99999, true, http.StatusUnauthorized},
		{"missing identity is unauthorized", 0, false, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			w := httptest.NewRecorder()
			newRouter(tc.uid, tc.setUser).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d, want %d (body=%q)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestNewUsersDefaultToNonAdmin(t *testing.T) {
	db := newRBACTestDB(t)
	repo := users.NewRepository(db)

	created, err := repo.Create("fresh@test.local", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	reloaded, err := repo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.IsAdmin() {
		t.Fatalf("newly registered user must default to non-admin, got role=%q", reloaded.Role)
	}
}
