package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, stats)
}
