package events

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

const (
	currentEventsDefaultLimit = 6
	currentEventsMaxLimit     = 6
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

func parseCurrentEventsLimit(raw string) (int, bool) {
	if raw == "" {
		return currentEventsDefaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > currentEventsMaxLimit {
		return 0, false
	}
	return n, true
}

func (h *AppHandler) handleGetCurrentEvents(c *gin.Context) {
	device := c.MustGet("device").(devices.Device)

	limit, ok := parseCurrentEventsLimit(c.Query("limit"))
	if !ok {
		server.RespondError(c, http.StatusBadRequest, "limit must be between 1 and 6")
		return
	}

	events, err := h.svc.GetCurrentAndUpcoming(c.Request.Context(), device.ID, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "unable to fetch events")
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.EventsList{Data: EventsToProto(events)})
}
