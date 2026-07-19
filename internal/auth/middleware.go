package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := ExtractBearerToken(c)
		if !ok {
			server.RespondError(c, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}

		claims, err := parseToken(tokenStr, accessTokenType)
		if err != nil {
			server.RespondError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}

		userID, err := claims.UserID()
		if err != nil {
			server.RespondError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}
