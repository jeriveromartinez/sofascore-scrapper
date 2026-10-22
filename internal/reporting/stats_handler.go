package reporting

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
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
		slog.Default().Error("reporting: get top events failed", slog.String("error", err.Error()))
		server.RespondError(c, http.StatusInternalServerError, "internal server error")
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.TopEventsResponse{Stats: EventStatsToProto(stats)})
}
