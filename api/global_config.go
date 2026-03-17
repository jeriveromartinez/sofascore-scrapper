package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, configs)
}

func handleAddGlobalConfig(c *gin.Context) {
	var req struct {
		TournamentIDs []uint `json:"tournament_ids" cbor:"tournament_ids"`
	}
	if err := parseCBORBody(c, &req); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	config, err := repository.SetGlobalTournamentConfig(req.TournamentIDs)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusCreated, config)
}

func handleRemoveGlobalConfig(c *gin.Context) {
	tournamentID, err := parseID(c.Param("tournamentId"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid tournament id"})
		return
	}

	if err := repository.RemoveGlobalTournamentConfig(tournamentID); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]string{"message": "tournament removed from global config"})
}
