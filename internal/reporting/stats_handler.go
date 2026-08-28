package reporting

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type StatsHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type StatsHandler struct {
	repo *Repository
}

func NewStatsHandler(repo *Repository) *StatsHandler {
	return &StatsHandler{repo: repo}
}

func (h *StatsHandler) RegisterRoutes(group *gin.RouterGroup, deps StatsHandlerDeps) {
	group.GET("/stats/top-events", deps.AuthMiddleware, h.handleTopEvents)
}

func (h *StatsHandler) handleTopEvents(c *gin.Context) {
	limitStr := c.Query("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	stats, err := h.repo.GetTopEvents(c.Request.Context(), limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.TopEventsResponse{Stats: EventStatsToProto(stats)})
}
