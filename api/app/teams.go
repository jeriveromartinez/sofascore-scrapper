package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/imageproxy"
)

type TeamController struct {
	Group *gin.RouterGroup
}

func (c *TeamController) LoadRoutes() {
	c.Group.GET("/teams/logo/:teamId", handleGetTeamLogo)
}

func handleGetTeamLogo(c *gin.Context) {
	teamIDStr := c.Param("teamId")
	teamID, err := strconv.ParseInt(teamIDStr, 10, 64)
	if err != nil || teamID <= 0 {
		common.RespondError(c, http.StatusBadRequest, "invalid team ID")
		return
	}

	localPath := imageproxy.TeamLogoLocalPath(teamID)

	storageDir, err := filepath.Abs(filepath.Join(imageproxy.StoragePath(), "teams"))
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "internal error")
		return
	}
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, "internal error")
		return
	}
	rel, relErr := filepath.Rel(storageDir, absPath)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		common.RespondError(c, http.StatusForbidden, "invalid path")
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		common.RespondError(c, http.StatusNotFound, "image not found")
		return
	}

	c.File(absPath)
}
