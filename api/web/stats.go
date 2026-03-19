package web

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type StatsController struct {
	Group *gin.RouterGroup
}

func (c *StatsController) LoadRoutes() {
	c.Group.GET("/stats/top-events", common.AuthMiddleware(), handleTopEvents)
}

func handleTopEvents(c *gin.Context) {
	limitStr := c.Query("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	stats, err := repository.GetTopEvents(limit)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.TopEventsResponse{Stats: common.EventStatsToProto(stats)})
}
