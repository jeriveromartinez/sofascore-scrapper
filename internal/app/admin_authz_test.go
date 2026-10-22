package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/auth"
	"github.com/jeriveromartinez/scores365-scrapper/internal/config"
	"github.com/jeriveromartinez/scores365-scrapper/internal/realtime"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newAuthzTestRouter(t *testing.T) (*gin.Engine, int) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}

	nonAdmin := users.User{Email: "regular@test.local", Password: "x", Role: users.RoleUser}
	if err := db.Create(&nonAdmin).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	t.Cleanup(mr.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("token service: %v", err)
	}

	router := NewRouter(db, redisClient, config.Config{JWTSecret: "this-is-a-test-secret-with-enough-length"}, tokens, nil, realtime.NewHub(), nil, nil)
	return router, int(nonAdmin.ID)
}

func TestNonAdminCannotListUsers(t *testing.T) {
	router, userID := newAuthzTestRouter(t)

	tokens, err := auth.NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	accessToken, err := tokens.GenerateAccessToken(uint(userID), "regular@test.local")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/web/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d; body=%q", w.Code, http.StatusForbidden, w.Body.String())
	}
}
