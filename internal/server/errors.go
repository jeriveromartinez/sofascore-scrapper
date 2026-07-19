package server

import "github.com/gin-gonic/gin"

func RespondError(c *gin.Context, status int, msg string) {
	c.JSON(status, map[string]string{"error": msg})
}
