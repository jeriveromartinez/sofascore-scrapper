package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	protobufBodyLimit    int64 = 1 << 20
	directUploadLimit    int64 = 200<<20 + 1<<20
	chunkUploadLimit     int64 = 10<<20 + 1<<20
	protobufReadDeadline       = 10 * time.Second
	uploadReadDeadline         = 15 * time.Minute
)

func BodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, readDeadline := classifyBodyLimit(c.Request.Method, c.Request.URL.Path)
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

func classifyBodyLimit(method, path string) (int64, time.Duration) {
	const (
		legacyUploadPath = "/api/web/v1/apk/upload"
		uploadsPath      = "/api/web/v1/apk/uploads/"
	)

	if path == legacyUploadPath+"/chunk" || strings.HasPrefix(path, legacyUploadPath+"/chunk/") {
		return chunkUploadLimit, uploadReadDeadline
	}
	if method == http.MethodPut && strings.HasPrefix(path, uploadsPath) {
		parts := strings.Split(strings.TrimPrefix(path, uploadsPath), "/")
		if len(parts) == 3 && parts[0] != "" && parts[1] == "chunks" && parts[2] != "" {
			return chunkUploadLimit, uploadReadDeadline
		}
	}
	if path == legacyUploadPath || strings.HasPrefix(path, legacyUploadPath+"/") {
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
