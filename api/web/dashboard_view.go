package web

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
)

func RegisterDashboardRoutes(router *gin.Engine) {
	frontendRoot := filepath.Clean("web/dist")
	indexPath := filepath.Join(frontendRoot, "index.html")

	if _, err := os.Stat(indexPath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("warning: could not stat dashboard index file: %v", err)
		}
		log.Printf("dashboard build not found at %s; serving API only", indexPath)
		return
	}

	log.Printf("serving dashboard from %s", frontendRoot)

	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			common.RespondError(c, http.StatusNotFound, "not found")
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