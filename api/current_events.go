package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type CurrentEventsController struct {
	Group *gin.RouterGroup
}

func (c *CurrentEventsController) LoadRoutes() {
	c.Group.GET("/current-events", appMiddleware(), handleGetCurrentEvents)
}

func handleGetCurrentEvents(c *gin.Context) {
	device := c.MustGet("device").(models.Device)
	limit := 6
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 && parsedLimit <= 6 {
			limit = parsedLimit
		}
	}

	events, err := repository.GetCurrentAndUpcomingEvents(device.ID, limit)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, events)
}
