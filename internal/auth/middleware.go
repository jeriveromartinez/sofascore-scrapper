package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
)

func AuthMiddleware(ts *TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := ExtractBearerToken(c)
		if !ok {
			server.RespondError(c, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}

		claims, err := ts.ParseAccessToken(tokenStr)
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
	}
}

// RequireAdmin must run after AuthMiddleware. It resolves the authenticated
// user and rejects the request unless the account holds the admin role.
func RequireAdmin(userRepo *users.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := GetUserID(c)
		if !ok {
			server.RespondError(c, http.StatusUnauthorized, "missing token")
			c.Abort()
			return
		}

		user, err := userRepo.GetByID(userID)
		if err != nil {
			server.RespondError(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}

		if !user.IsAdmin() {
			server.RespondError(c, http.StatusForbidden, "admin privileges required")
			c.Abort()
			return
		}
	}
}
