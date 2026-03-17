package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
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

func parseCBORBody(c *gin.Context, v any) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return cbor.NewDecoder(bytes.NewReader(body)).Decode(v)
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

func appMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		db, err := database.GetDB()
		if err != nil {
			respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "you are lost"})
			c.Abort()
			return
		}

		var device models.Device
		if err := db.Where("token = ?", c.GetHeader("APP-XIPTV")).First(&device).Error; err != nil {
			respondCBOR(c, http.StatusUnauthorized, map[string]string{"error": "you are lost"})
			c.Abort()
			return
		}

		c.Set("device", device)
		c.Next()
	}
}

func parseID(idStr string) (uint, error) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func corsMiddleware() gin.HandlerFunc {
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

// Start starts the HTTP API server.
func Start(addr string) {
	router := gin.New()
	router.Use(corsMiddleware(), gin.Logger(), gin.Recovery())

	v1 := router.Group("/api/v1")

	(&EventController{Group: v1}).LoadRoutes()
	(&UserController{Group: v1}).LoadRoutes()
	(&DeviceController{Group: v1}).LoadRoutes()
	(&PlaybackController{Group: v1}).LoadRoutes()
	(&StatsController{Group: v1}).LoadRoutes()
	(&ApkController{Group: v1}).LoadRoutes()
	(&TeamController{Group: v1}).LoadRoutes()
	(&TournamentController{Group: v1}).LoadRoutes()
	(&DeviceTournamentController{Group: v1}).LoadRoutes()
	(&GlobalConfigController{Group: v1}).LoadRoutes()
	(&CurrentEventsController{Group: v1}).LoadRoutes()

	registerDashboardRoutes(router)

	log.Printf("API server listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}

func registerDashboardRoutes(router *gin.Engine) {
	frontendRoot := filepath.Clean("web/dist")
	indexPath := filepath.Join(frontendRoot, "index.html")

	if _, err := os.Stat(indexPath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("warning: could not stat dashboard index file: %v", err)
		}
		log.Printf("dashboard build not found at %s; serving API only", indexPath)
		return
	}

	log.Printf("serving dashboard from %s", frontendRoot)

	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			respondCBOR(c, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		relPath := strings.TrimLeft(filepath.Clean(c.Request.URL.Path), "/\\")
		requestedPath := filepath.Join(frontendRoot, relPath)
		if info, err := os.Stat(requestedPath); err == nil && !info.IsDir() {
			c.File(requestedPath)
			return
		}

		c.File(indexPath)
	})
}
