package common

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
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

func getJWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("WARNING: JWT_SECRET env var is not set; using insecure default. Set JWT_SECRET in production.")
	return []byte("changeme-please-set-JWT_SECRET-env")
}

func GenerateAccessToken(userID uint, username string) (string, error) {
	return generateToken(userID, username, accessTokenType, accessTokenTTL, "")
}

func GenerateRefreshToken(userID uint, username string) (string, string, time.Time, error) {
	tokenID, err := randomTokenID()
	if err != nil {
		return "", "", time.Time{}, err
	}

	expiresAt := time.Now().Add(refreshTokenTTL)
	token, err := generateToken(userID, username, refreshTokenType, refreshTokenTTL, tokenID)
	if err != nil {
		return "", "", time.Time{}, err
	}

	return token, tokenID, expiresAt, nil
}

func GenerateTokenPair(userID uint, username string) (string, string, string, time.Time, error) {
	accessToken, err := GenerateAccessToken(userID, username)
	if err != nil {
		return "", "", "", time.Time{}, err
	}

	refreshToken, tokenID, expiresAt, err := GenerateRefreshToken(userID, username)
	if err != nil {
		return "", "", "", time.Time{}, err
	}

	return accessToken, refreshToken, tokenID, expiresAt, nil
}

func ParseRefreshToken(tokenStr string) (*TokenClaims, error) {
	return parseToken(tokenStr, refreshTokenType)
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := ExtractBearerToken(c)
		if !ok {
			RespondError(c, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}

		claims, err := parseToken(tokenStr, accessTokenType)
		if err != nil {
			RespondError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}

		userID, err := claims.UserID()
		if err != nil {
			RespondError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
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

func generateToken(userID uint, username, tokenType string, ttl time.Duration, tokenID string) (string, error) {
	now := time.Now()
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
	return token.SignedString(getJWTSecret())
}

func parseToken(tokenStr, expectedType string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return getJWTSecret(), nil
	})
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

func GetUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get(userIDKey)
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

func AppMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		db, err := database.GetDB()
		if err != nil {
			RespondError(c, http.StatusUnauthorized, "you are lost")
			c.Abort()
			return
		}

		var device models.Device
		if err := db.Where("token = ?", c.GetHeader("APP-XIPTV")).First(&device).Error; err != nil {
			RespondError(c, http.StatusUnauthorized, "you are lost")
			c.Abort()
			return
		}

		c.Set("device", device)
		c.Next()
	}
}

func ParseID(idStr string) (uint, error) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "*")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
