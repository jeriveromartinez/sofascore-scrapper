package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterDashboardRoutes(router *gin.Engine) {
	frontendRoot := filepath.Clean("web/dist")
	indexPath := filepath.Join(frontendRoot, "index.html")

	if _, err := os.Stat(indexPath); err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("could not stat dashboard index file", slog.String("error", err.Error()))
		}
		slog.Warn("dashboard build not found, serving API only", slog.String("path", indexPath))
		return
	}

	slog.Info("serving dashboard", slog.String("path", frontendRoot))

	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			RespondError(c, http.StatusNotFound, "not found")
			return
		}

		relPath := strings.TrimLeft(filepath.Clean(c.Request.URL.Path), "/\\")
		requestedPath := filepath.Join(frontendRoot, relPath)
		if info, err := os.Stat(requestedPath); err == nil && !info.IsDir() {
			c.File(requestedPath)
			return
		}

		c.File(indexPath)
	})
}
