package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/platform/observability"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Expose-Headers", "*")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		logger := observability.FromContext(c.Request.Context())

		attrs := []slog.Attr{
			slog.Int("status", c.Writer.Status()),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("latency", latency.String()),
			slog.String("client_ip", c.ClientIP()),
		}
		if rawQuery != "" {
			attrs = append(attrs, slog.String("query", redactQuery(rawQuery)))
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "request completed", attrs...)
	}
}

// sensitiveQueryParams lists the query string parameters whose value
// must never reach the logs.
var sensitiveQueryParams = map[string]bool{
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"password":      true,
	"secret":        true,
	"api_key":       true,
	"apikey":        true,
	"code":          true,
	"authorization": true,
}

// redactQuery replaces the value of every sensitive query parameter
// with a placeholder. The WebSocket upgrade accepts the device
// credential in ?token=, so the raw query string is a credential leak
// in the access log.
func redactQuery(raw string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "<unparseable>"
	}
	for key := range values {
		if sensitiveQueryParams[strings.ToLower(key)] {
			values.Set(key, "REDACTED")
		}
	}
	return values.Encode()
}
