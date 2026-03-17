package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type StatsController struct {
	Group *gin.RouterGroup
}

func (c *StatsController) LoadRoutes() {
	c.Group.GET("/stats/top-events", authMiddleware(), handleTopEvents)
}

func handleTopEvents(c *gin.Context) {
	limitStr := c.Query("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	stats, err := repository.GetTopEvents(limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.TopEventsResponse{Stats: eventStatsToProto(stats)})
}
