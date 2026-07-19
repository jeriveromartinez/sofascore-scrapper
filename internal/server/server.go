package server

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	return gin.New()
}

func ReadinessMiddleware(ready *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ready != nil && !ready.Load() {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		c.Next()
	}
}
