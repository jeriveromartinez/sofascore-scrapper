package web

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type PlaybackController struct {
	Group *gin.RouterGroup
}

func (c *PlaybackController) LoadRoutes() {
	c.Group.GET("/playback", common.AuthMiddleware(), handleGetLogPlayback)
}

func handleGetLogPlayback(c *gin.Context) {
	page := 1
	limit := 10

	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			common.RespondError(c, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			common.RespondError(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	stats, err := repository.GetList(page, limit)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	total := repository.TotalCount()

	common.RespondProto(c, http.StatusOK, common.PlaybackListToProto(stats, total))
}
