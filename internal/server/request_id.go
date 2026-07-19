package server

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/platform/observability"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Header("X-Request-ID", id)

		ctx := observability.WithRequest(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)

		c.Set("request_id", id)
		c.Set("logger", observability.FromContext(ctx))

		c.Next()
	}
}
