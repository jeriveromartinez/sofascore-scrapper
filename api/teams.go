package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/imageproxy"
)

// TeamController serves proxied team logo images.
type TeamController struct {
	Group *gin.RouterGroup
}

func (c *TeamController) LoadRoutes() {
	c.Group.GET("/teams/logo/:teamId", handleGetTeamLogo)
}

// handleGetTeamLogo serves a locally cached team logo image.
// The teamId path parameter must be a positive integer.
func handleGetTeamLogo(c *gin.Context) {
	teamIDStr := c.Param("teamId")
	teamID, err := strconv.ParseInt(teamIDStr, 10, 64)
	if err != nil || teamID <= 0 {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid team ID"})
		return
	}

	localPath := imageproxy.TeamLogoLocalPath(teamID)

	// Security: confirm the resolved path stays inside the designated storage directory.
	storageDir, err := filepath.Abs(filepath.Join(imageproxy.StoragePath(), "teams"))
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	rel, relErr := filepath.Rel(storageDir, absPath)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		respondCBOR(c, http.StatusForbidden, map[string]string{"error": "invalid path"})
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		respondCBOR(c, http.StatusNotFound, map[string]string{"error": "image not found"})
		return
	}

	c.File(absPath)
}
