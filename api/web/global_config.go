package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type GlobalConfigController struct {
	Group *gin.RouterGroup
}

func (c *GlobalConfigController) LoadRoutes() {
	c.Group.GET("/global-tournament-config", common.AuthMiddleware(), handleGetGlobalConfig)
	c.Group.POST("/global-tournament-config", common.AuthMiddleware(), handleAddGlobalConfig)
	c.Group.DELETE("/global-tournament-config/:tournamentId", common.AuthMiddleware(), handleRemoveGlobalConfig)
}

func handleGetGlobalConfig(c *gin.Context) {
	configs, err := repository.GetGlobalTournamentConfig()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.GlobalTournamentConfigList{Configs: common.GlobalConfigsToProto(configs)})
}

func handleAddGlobalConfig(c *gin.Context) {
	var req pb.SetTournamentIdsRequest
	if err := common.ParseProtoBody(c, &req); err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	configs, err := repository.SetGlobalTournamentConfig(ids)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusCreated, &pb.GlobalTournamentConfigList{Configs: common.GlobalConfigPtrsToProto(configs)})
}

func handleRemoveGlobalConfig(c *gin.Context) {
	tournamentID, err := common.ParseID(c.Param("tournamentId"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid tournament id")
		return
	}

	if err := repository.RemoveGlobalTournamentConfig(tournamentID); err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from global config"})
}
