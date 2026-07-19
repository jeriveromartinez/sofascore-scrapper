package common

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

func GenerateAccessToken(userID uint, username string) (string, error) {
	return auth.GenerateAccessToken(userID, username)
}

func GenerateRefreshToken(userID uint, username string) (string, string, time.Time, error) {
	return auth.GenerateRefreshToken(userID, username)
}

func GenerateTokenPair(userID uint, username string) (string, string, string, time.Time, error) {
	return auth.GenerateTokenPair(userID, username)
}

type TokenClaims = auth.TokenClaims

func ParseRefreshToken(tokenStr string) (*TokenClaims, error) {
	return auth.ParseRefreshToken(tokenStr)
}

func ExtractBearerToken(c *gin.Context) (string, bool) {
	return auth.ExtractBearerToken(c)
}

func GetUserID(c *gin.Context) (uint, bool) {
	return auth.GetUserID(c)
}

func AuthMiddleware() gin.HandlerFunc {
	return auth.AuthMiddleware()
}

func AppMiddleware() gin.HandlerFunc {
	return devices.AppMiddleware()
}

func CorsMiddleware() gin.HandlerFunc {
	return server.CORS()
}

func ParseID(idStr string) (uint, error) {
	return server.ParseID(idStr)
}
