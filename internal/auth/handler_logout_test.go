//go:build integration

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newLogoutTestDB wires the auth schema (RefreshToken) and the users
// schema (User) into a single in-memory DB and returns a TokenService
// primed with a known secret so handler tests can mint refresh tokens
// deterministically.
func newLogoutTestDB(t *testing.T) (*gorm.DB, *TokenService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&RefreshToken{}); err != nil {
		t.Fatalf("migrate RefreshToken: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	ts, err := NewTokenService("this-is-a-test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	return db, ts
}

// logoutRouter returns a gin engine that mimics the production logout
// route: it injects the userID into the request context (as
// AuthMiddleware would) and dispatches the handler under test.
func logoutRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/users/logout", func(c *gin.Context) {
		// Production middleware would do this; we inline it so the test
		// does not need to mint an access token just to set the user.
		c.Set(userIDKey, uint(7))
		h.handleLogout(c)
	})
	return r
}

// seedPair inserts two active refresh tokens for the same user so we can
// verify that revoking one does not cascade to the other.
func seedPair(t *testing.T, db *gorm.DB, userID uint, tokenIDA, tokenIDB string) {
	t.Helper()
	exp := time.Now().Add(24 * time.Hour)
	for _, id := range []string{tokenIDA, tokenIDB} {
		if err := db.Create(&RefreshToken{UserID: userID, TokenID: id, ExpiresAt: exp}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
}

func TestHandleLogout_NoHeaderNoAllQuery_Returns400AndKeepsBothTokens(t *testing.T) {
	db, ts := newLogoutTestDB(t)
	repo := NewAuthRepository(db)
	h := NewAuthHandler(repo, nil, ts, nil)

	seedPair(t, db, 7, "tok-a", "tok-b")
	r := logoutRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/users/logout", bytes.NewBufferString(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (body=%q)", w.Code, w.Body.String())
	}
	// Both tokens must remain active — the handler must not silently
	// fall through to the revoke-all branch.
	if !isActive(db, 7, "tok-a") {
		t.Fatal("tok-a should still be active after 400 response")
	}
	if !isActive(db, 7, "tok-b") {
		t.Fatal("tok-b should still be active after 400 response")
	}
}

func TestHandleLogout_WithMatchingRefreshHeader_RevokesOnlyThatToken(t *testing.T) {
	db, ts := newLogoutTestDB(t)
	repo := NewAuthRepository(db)
	h := NewAuthHandler(repo, nil, ts, nil)

	seedPair(t, db, 7, "tok-a", "tok-b")

	// Mint a real refresh token for user 7 so the handler's parse path runs.
	refreshToken, tokenID, _, err := ts.GenerateRefreshToken(7, "u@x.com")
	if err != nil {
		t.Fatalf("mint refresh: %v", err)
	}
	// Persist it so RevokeRefreshToken has a row to update.
	if err := repo.SaveRefreshToken(7, tokenID, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("save refresh: %v", err)
	}

	r := logoutRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/users/logout", bytes.NewBufferString(""))
	req.Header.Set("X-Refresh-Token", refreshToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (body=%q)", w.Code, w.Body.String())
	}
	// The minted token is revoked, but the two seeded tokens survive.
	if isActive(db, 7, tokenID) {
		t.Fatalf("minted token %s should be revoked", tokenID)
	}
	if !isActive(db, 7, "tok-a") {
		t.Fatal("tok-a should still be active after targeted logout")
	}
	if !isActive(db, 7, "tok-b") {
		t.Fatal("tok-b should still be active after targeted logout")
	}
}

func TestHandleLogout_WithAllQuery_RevokesEveryTokenForUser(t *testing.T) {
	db, ts := newLogoutTestDB(t)
	repo := NewAuthRepository(db)
	h := NewAuthHandler(repo, nil, ts, nil)

	seedPair(t, db, 7, "tok-a", "tok-b")
	// Add a third token from a different user to verify scoping.
	if err := db.Create(&RefreshToken{
		UserID: 99, TokenID: "tok-other", ExpiresAt: time.Now().Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	r := logoutRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/users/logout?all=true", bytes.NewBufferString(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (body=%q)", w.Code, w.Body.String())
	}
	if isActive(db, 7, "tok-a") {
		t.Fatal("tok-a should be revoked with ?all=true")
	}
	if isActive(db, 7, "tok-b") {
		t.Fatal("tok-b should be revoked with ?all=true")
	}
	// Tokens for other users must NOT be touched.
	if !isActive(db, 99, "tok-other") {
		t.Fatal("other user's token must remain active")
	}
}

func TestHandleLogout_RefreshHeaderFromDifferentUser_FallsThroughTo400(t *testing.T) {
	db, ts := newLogoutTestDB(t)
	repo := NewAuthRepository(db)
	h := NewAuthHandler(repo, nil, ts, nil)

	seedPair(t, db, 7, "tok-a", "tok-b")

	// Mint a refresh token for user 99, but the route is authenticated as
	// user 7. The handler must NOT revoke user 7's tokens, and it must NOT
	// also accidentally accept the foreign token — it should fall through
	// to the 400 branch.
	foreignToken, foreignID, _, err := ts.GenerateRefreshToken(99, "x@x.com")
	if err != nil {
		t.Fatalf("mint foreign: %v", err)
	}
	if err := repo.SaveRefreshToken(99, foreignID, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("save foreign: %v", err)
	}

	r := logoutRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/users/logout", bytes.NewBufferString(""))
	req.Header.Set("X-Refresh-Token", foreignToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (body=%q)", w.Code, w.Body.String())
	}
	// No tokens for user 7 were touched.
	if !isActive(db, 7, "tok-a") {
		t.Fatal("tok-a should still be active")
	}
	if !isActive(db, 7, "tok-b") {
		t.Fatal("tok-b should still be active")
	}
	if !isActive(db, 99, foreignID) {
		t.Fatal("foreign token must remain active")
	}
}
