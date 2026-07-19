package events

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type LogoHandler struct{}

func NewLogoHandler() *LogoHandler {
	return &LogoHandler{}
}

func (h *LogoHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/teams/logo/:teamId", h.handle)
}

func (h *LogoHandler) handle(c *gin.Context) {
	teamIDStr := c.Param("teamId")
	teamID, err := strconv.ParseInt(teamIDStr, 10, 64)
	if err != nil || teamID <= 0 {
		server.RespondError(c, http.StatusBadRequest, "invalid team ID")
		return
	}

	localPath := TeamLogoLocalPath(teamID)

	storageDir, err := filepath.Abs(filepath.Join(StoragePath(), "teams"))
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "internal error")
		return
	}
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "internal error")
		return
	}
	rel, relErr := filepath.Rel(storageDir, absPath)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		server.RespondError(c, http.StatusForbidden, "invalid path")
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		server.RespondError(c, http.StatusNotFound, "image not found")
		return
	}

	c.File(absPath)
}
