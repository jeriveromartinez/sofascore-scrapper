package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const userIDKey = "userID"

func getJWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("WARNING: JWT_SECRET env var is not set; using insecure default. Set JWT_SECRET in production.")
	return []byte("changeme-please-set-JWT_SECRET-env")
}

func respondCBOR(c *gin.Context, status int, v any) {
	data, err := cbor.Marshal(v)
	if err != nil {
		c.String(http.StatusInternalServerError, "encoding error")
		return
	}
	c.Data(status, "application/cbor", data)
}

func bindBody(c *gin.Context, v any) error {
	ct := c.GetHeader("Content-Type")
	if strings.Contains(ct, "application/cbor") {
		return cbor.NewDecoder(c.Request.Body).Decode(v)
	}
	return json.NewDecoder(c.Request.Body).Decode(v)
}

func generateToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "missing token"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return getJWTSecret(), nil
		})
		if err != nil || !token.Valid {
			respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			c.Abort()
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			c.Abort()
			return
		}
		var userID uint
		switch v := claims["sub"].(type) {
		case float64:
			userID = uint(v)
		case uint:
			userID = v
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

func getUserID(c *gin.Context) uint {
	v, _ := c.Get(userIDKey)
	id, _ := v.(uint)
	return id
}

// Start starts the HTTP API server.
func Start(addr string) {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	v1 := router.Group("/api/v1")

	(&EventController{Group: v1}).LoadRoutes()
	(&UserController{Group: v1}).LoadRoutes()
	(&DeviceController{Group: v1}).LoadRoutes()
	(&PlaybackController{Group: v1}).LoadRoutes()
	(&StatsController{Group: v1}).LoadRoutes()
	(&ApkController{Group: v1}).LoadRoutes()

	log.Printf("API server listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}
