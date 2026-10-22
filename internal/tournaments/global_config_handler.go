package tournaments

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
)

type GlobalConfigHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type GlobalConfigHandler struct {
	repo     *GlobalConfigRepository
	onChange func(context.Context) error
}

func NewGlobalConfigHandler(repo *GlobalConfigRepository) *GlobalConfigHandler {
	return &GlobalConfigHandler{repo: repo}
}

func (h *GlobalConfigHandler) SetOnChange(fn func(context.Context) error) {
	h.onChange = fn
}

func (h *GlobalConfigHandler) RegisterRoutes(group *gin.RouterGroup, deps GlobalConfigHandlerDeps) {
	group.GET("/global-tournament-config", deps.AuthMiddleware, h.handleGet)
	group.POST("/global-tournament-config", deps.AuthMiddleware, h.handleAdd)
	group.DELETE("/global-tournament-config/:tournamentId", deps.AuthMiddleware, h.handleRemove)
}

func (h *GlobalConfigHandler) handleGet(c *gin.Context) {
	configs, err := h.repo.GetAll(c.Request.Context())
	if err != nil {
		respondInternalError(c, "list global tournament config failed", err)
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.GlobalTournamentConfigList{Configs: GlobalConfigsToProto(configs)})
}

func (h *GlobalConfigHandler) handleAdd(c *gin.Context) {
	var req pb.SetTournamentIdsRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	configs, err := h.repo.Set(c.Request.Context(), ids)
	if err != nil {
		respondInternalError(c, "set global tournament config failed", err)
		return
	}
	notifyChange(c, h.onChange)
	server.RespondProto(c, http.StatusCreated, &pb.GlobalTournamentConfigList{Configs: GlobalConfigPtrsToProto(configs)})
}

func (h *GlobalConfigHandler) handleRemove(c *gin.Context) {
	tournamentID, err := server.ParseID(c.Param("tournamentId"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid tournament id")
		return
	}

	if err := h.repo.Remove(c.Request.Context(), tournamentID); err != nil {
		respondInternalError(c, "remove global tournament config failed", err)
		return
	}
	notifyChange(c, h.onChange)
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from global config"})
}
