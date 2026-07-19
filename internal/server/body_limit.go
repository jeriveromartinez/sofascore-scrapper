package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	protobufBodyLimit  int64 = 1 << 20
	directUploadLimit  int64 = 200<<20 + 1<<20
	chunkUploadLimit   int64 = 10<<20 + 1<<20
)

func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := classifyBodyLimit(c.Request.URL.Path)
		if c.Request.ContentLength > limit {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func classifyBodyLimit(path string) int64 {
	if strings.HasPrefix(path, "/api/web/v1/apk/upload/chunk") {
		return chunkUploadLimit
	}
	if strings.HasPrefix(path, "/api/web/v1/apk/upload") {
		return directUploadLimit
	}
	return protobufBodyLimit
}
