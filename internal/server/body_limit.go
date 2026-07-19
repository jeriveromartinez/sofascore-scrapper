package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	protobufBodyLimit  int64 = 1 << 20
	directUploadLimit  int64 = 200<<20 + 1<<20
	chunkUploadLimit   int64 = 10<<20 + 1<<20
	protobufReadDeadline     = 10 * time.Second
	uploadReadDeadline       = 15 * time.Minute
)

func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, readDeadline := classifyBodyLimit(c.Request.URL.Path)
		if readDeadline > 0 {
			setReadDeadline(c, readDeadline)
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func classifyBodyLimit(path string) (int64, time.Duration) {
	if strings.HasPrefix(path, "/api/web/v1/apk/upload/chunk") {
		return chunkUploadLimit, uploadReadDeadline
	}
	if strings.HasPrefix(path, "/api/web/v1/apk/upload") {
		return directUploadLimit, uploadReadDeadline
	}
	return protobufBodyLimit, protobufReadDeadline
}

func setReadDeadline(c *gin.Context, dur time.Duration) {
	rc := http.NewResponseController(c.Writer)
	if rc != nil {
		rc.SetReadDeadline(time.Now().Add(dur))
	}
}
