package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type GlobalConfigController struct {
	Group *gin.RouterGroup
}

func (c *GlobalConfigController) LoadRoutes() {
	c.Group.GET("/global-tournament-config", authMiddleware(), handleGetGlobalConfig)
	c.Group.POST("/global-tournament-config", authMiddleware(), handleAddGlobalConfig)
	c.Group.DELETE("/global-tournament-config/:tournamentId", authMiddleware(), handleRemoveGlobalConfig)
}

func handleGetGlobalConfig(c *gin.Context) {
	configs, err := repository.GetGlobalTournamentConfig()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.GlobalTournamentConfigList{Configs: globalConfigsToProto(configs)})
}

func handleAddGlobalConfig(c *gin.Context) {
	var req pb.SetTournamentIdsRequest
	if err := parseProtoBody(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	configs, err := repository.SetGlobalTournamentConfig(ids)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusCreated, &pb.GlobalTournamentConfigList{Configs: globalConfigPtrsToProto(configs)})
}

func handleRemoveGlobalConfig(c *gin.Context) {
	tournamentID, err := parseID(c.Param("tournamentId"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tournament id")
		return
	}

	if err := repository.RemoveGlobalTournamentConfig(tournamentID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from global config"})
}
