package devices

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"gorm.io/gorm"
)

func AppMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var device Device
		if err := db.Where("token = ?", c.GetHeader("APP-XIPTV")).First(&device).Error; err != nil {
			server.RespondError(c, http.StatusUnauthorized, "you are lost")
			c.Abort()
			return
		}

		c.Set("device", device)
		c.Next()
	}
}
