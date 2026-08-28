package auth

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestNewTokenService_RejectsEmptySecret(t *testing.T) {
	_, err := NewTokenService("")
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestNewTokenService_RejectsWhitespaceSecret(t *testing.T) {
	_, err := NewTokenService("   ")
	if err == nil {
		t.Fatal("expected error for whitespace secret")
	}
}

func TestNewTokenService_RejectsShortSecret(t *testing.T) {
	// 31 chars: one below the policy minimum.
	_, err := NewTokenService("short-secret-but-31-chars-len!")
	if !errors.Is(err, ErrJWTSecretTooShort) {
		t.Fatalf("err=%v, want ErrJWTSecretTooShort", err)
	}
}

func TestNewTokenService_AcceptsValidSecret(t *testing.T) {
	ts, err := NewTokenService("my-secret-key-with-enough-length-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	if ts == nil {
		t.Fatal("TokenService is nil")
	}
}

func TestGenerateAndParseAccessToken(t *testing.T) {
	ts, err := NewTokenService("test-secret-with-enough-length-for-suite")
	if err != nil {
		t.Fatal(err)
	}

	token, err := ts.GenerateAccessToken(42, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	claims, err := ts.ParseAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Username != "user@example.com" {
		t.Fatalf("Username=%q", claims.Username)
	}
	userID, err := claims.UserID()
	if err != nil {
		t.Fatal(err)
	}
	if userID != 42 {
		t.Fatalf("UserID=%d", userID)
	}
	if claims.Type != accessTokenType {
		t.Fatalf("Type=%s", claims.Type)
	}
}

func TestGenerateAndParseRefreshToken(t *testing.T) {
	ts, err := NewTokenService("test-secret-with-enough-length-for-suite")
	if err != nil {
		t.Fatal(err)
	}

	token, tokenID, expiresAt, err := ts.GenerateRefreshToken(42, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || tokenID == "" {
		t.Fatal("empty token or tokenID")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expires in the past")
	}

	claims, err := ts.ParseRefreshToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Type != refreshTokenType {
		t.Fatalf("Type=%s", claims.Type)
	}
	if claims.ID != tokenID {
		t.Fatalf("ID mismatch: got=%s want=%s", claims.ID, tokenID)
	}
}

func TestGenerateTokenPair(t *testing.T) {
	ts, err := NewTokenService("test-secret-with-enough-length-for-suite")
	if err != nil {
		t.Fatal(err)
	}

	accessToken, refreshToken, tokenID, expiresAt, err := ts.GenerateTokenPair(1, "test@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if accessToken == "" || refreshToken == "" || tokenID == "" {
		t.Fatal("empty token values")
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expires in the past")
	}
}

func TestParseAccessToken_RejectsRefreshToken(t *testing.T) {
	ts, err := NewTokenService("test-secret-with-enough-length-for-suite")
	if err != nil {
		t.Fatal(err)
	}

	token, _, _, err := ts.GenerateRefreshToken(1, "test@test.com")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ts.ParseAccessToken(token)
	if err == nil {
		t.Fatal("expected error parsing refresh token as access token")
	}
}

func TestParseRefreshToken_RejectsAccessToken(t *testing.T) {
	ts, err := NewTokenService("test-secret-with-enough-length-for-suite")
	if err != nil {
		t.Fatal(err)
	}

	token, err := ts.GenerateAccessToken(1, "test@test.com")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ts.ParseRefreshToken(token)
	if err == nil {
		t.Fatal("expected error parsing access token as refresh token")
	}
}

func TestParseToken_RejectsWrongSecret(t *testing.T) {
	ts1, _ := NewTokenService("secret-a-with-enough-length-for-tests")
	ts2, _ := NewTokenService("secret-b-with-enough-length-for-tests")

	token, err := ts1.GenerateAccessToken(1, "test@test.com")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ts2.ParseAccessToken(token)
	if err == nil {
		t.Fatal("expected error with wrong secret")
	}
}

func TestParseToken_RejectsHS384(t *testing.T) {
	claims := TokenClaims{
		Username: "test@test.com",
		Type:     accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "1",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	signed, err := token.SignedString([]byte("test-secret-with-enough-length-for-suite"))
	if err != nil {
		t.Fatal(err)
	}

	ts, _ := NewTokenService("test-secret-with-enough-length-for-suite")
	_, err = ts.ParseAccessToken(signed)
	if err == nil {
		t.Fatal("expected HS384 token to be rejected")
	}
}

func TestParseToken_RejectsHS512(t *testing.T) {
	claims := TokenClaims{
		Username: "test@test.com",
		Type:     accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "1",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := token.SignedString([]byte("test-secret-with-enough-length-for-suite"))
	if err != nil {
		t.Fatal(err)
	}

	ts, _ := NewTokenService("test-secret-with-enough-length-for-suite")
	_, err = ts.ParseAccessToken(signed)
	if err == nil {
		t.Fatal("expected HS512 token to be rejected")
	}
}

func TestTokenService_UsesFixedClock(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := &TokenService{secret: []byte("test-secret-with-enough-length-for-suite"), now: func() time.Time { return fixedTime }}

	token, err := ts.GenerateAccessToken(1, "test@test.com")
	if err != nil {
		t.Fatal(err)
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := &TokenClaims{}
	_, err = parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return ts.secret, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !claims.IssuedAt.Time.Equal(fixedTime) {
		t.Fatalf("IssuedAt=%v want=%v", claims.IssuedAt.Time, fixedTime)
	}
	if !claims.ExpiresAt.Time.Equal(fixedTime.Add(accessTokenTTL)) {
		t.Fatalf("ExpiresAt=%v want=%v", claims.ExpiresAt.Time, fixedTime.Add(accessTokenTTL))
	}
}

func TestExtractBearerToken(t *testing.T) {
	c := createTestContext("Bearer my-token")
	token, ok := ExtractBearerToken(c)
	if !ok {
		t.Fatal("expected to extract token")
	}
	if token != "my-token" {
		t.Fatalf("token=%q", token)
	}
}

func TestExtractBearerToken_Missing(t *testing.T) {
	c := createTestContext("")
	_, ok := ExtractBearerToken(c)
	if ok {
		t.Fatal("expected failure")
	}
}

func TestExtractBearerToken_WrongPrefix(t *testing.T) {
	c := createTestContext("Basic my-token")
	_, ok := ExtractBearerToken(c)
	if ok {
		t.Fatal("expected failure")
	}
}

func createTestContext(authHeader string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	c.Request = req
	return c
}
