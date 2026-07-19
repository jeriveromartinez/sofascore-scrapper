package events

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type AppHandlerDeps struct {
	AppMiddleware gin.HandlerFunc
}

type AppHandler struct {
	svc *Service
}

func NewAppHandler(svc *Service) *AppHandler {
	return &AppHandler{svc: svc}
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

	events, err := h.svc.GetCurrentAndUpcoming(c.Request.Context(), device.ID, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "unable to fetch events")
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.EventsList{Data: EventsToProto(events)})
}
