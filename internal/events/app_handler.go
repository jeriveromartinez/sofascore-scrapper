package events

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type AppHandlerDeps struct {
	AppMiddleware gin.HandlerFunc
}

type AppHandler struct {
	repo *Repository
}

func NewAppHandler(repo *Repository) *AppHandler {
	return &AppHandler{repo: repo}
}

func (h *AppHandler) RegisterRoutes(group *gin.RouterGroup, deps AppHandlerDeps) {
	group.GET("/current-events", deps.AppMiddleware, h.handleGetCurrentEvents)
}

func (h *AppHandler) handleGetCurrentEvents(c *gin.Context) {
	device := c.MustGet("device").(devices.Device)
	limit := 6
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 && parsedLimit <= 6 {
			limit = parsedLimit
		}
	}

	events, err := h.repo.GetCurrentAndUpcoming(device.ID, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.EventsList{Data: EventsToProto(events)})
}
