package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
)

const userIDKey = "userID"

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
	accessTokenTTL   = time.Hour
	refreshTokenTTL  = 7 * 24 * time.Hour
)

type TokenClaims struct {
	Username string `json:"username,omitempty"`
	Type     string `json:"type"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
	now    func() time.Time
}

func NewTokenService(secret string) (*TokenService, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, config.ErrJWTSecretRequired
	}
	return &TokenService{secret: []byte(secret), now: time.Now}, nil
}

func (ts *TokenService) GenerateAccessToken(userID uint, username string) (string, error) {
	return ts.generateToken(userID, username, accessTokenType, accessTokenTTL, "")
}

func (ts *TokenService) GenerateRefreshToken(userID uint, username string) (string, string, time.Time, error) {
	tokenID, err := randomTokenID()
	if err != nil {
		return "", "", time.Time{}, err
	}

	expiresAt := ts.now().Add(refreshTokenTTL)
	token, err := ts.generateToken(userID, username, refreshTokenType, refreshTokenTTL, tokenID)
	if err != nil {
		return "", "", time.Time{}, err
	}

	return token, tokenID, expiresAt, nil
}

func (ts *TokenService) GenerateTokenPair(userID uint, username string) (string, string, string, time.Time, error) {
	accessToken, err := ts.GenerateAccessToken(userID, username)
	if err != nil {
		return "", "", "", time.Time{}, err
	}

	refreshToken, tokenID, expiresAt, err := ts.GenerateRefreshToken(userID, username)
	if err != nil {
		return "", "", "", time.Time{}, err
	}

	return accessToken, refreshToken, tokenID, expiresAt, nil
}

func (ts *TokenService) ParseAccessToken(tokenStr string) (*TokenClaims, error) {
	return ts.parseToken(tokenStr, accessTokenType)
}

func (ts *TokenService) ParseRefreshToken(tokenStr string) (*TokenClaims, error) {
	return ts.parseToken(tokenStr, refreshTokenType)
}

func ExtractBearerToken(c *gin.Context) (string, bool) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	return strings.TrimPrefix(authHeader, "Bearer "), true
}

func (c *TokenClaims) UserID() (uint, error) {
	parsedID, err := strconv.ParseUint(c.Subject, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(parsedID), nil
}

func GetUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get(userIDKey)
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

func (ts *TokenService) generateToken(userID uint, username, tokenType string, ttl time.Duration, tokenID string) (string, error) {
	now := ts.now()
	claims := TokenClaims{
		Username: username,
		Type:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        tokenID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ts.secret)
}

func (ts *TokenService) parseToken(tokenStr, expectedType string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return ts.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || claims.Type != expectedType {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

func randomTokenID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
