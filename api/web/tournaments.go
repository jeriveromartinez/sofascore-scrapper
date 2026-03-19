package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type TournamentController struct {
	Group *gin.RouterGroup
}

func (c *TournamentController) LoadRoutes() {
	c.Group.GET("/tournaments", common.AuthMiddleware(), handleGetTournaments)
	c.Group.GET("/tournaments/:id", common.AuthMiddleware(), handleGetTournament)
	c.Group.POST("/tournaments", common.AuthMiddleware(), handleCreateTournament)
	c.Group.PUT("/tournaments/:id", common.AuthMiddleware(), handleUpdateTournament)
	c.Group.DELETE("/tournaments/:id", common.AuthMiddleware(), handleDeleteTournament)
}

func handleGetTournaments(c *gin.Context) {
	tournaments, err := repository.GetAllTournaments()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.TournamentList{Tournaments: common.TournamentsToProto(tournaments)})
}

func handleGetTournament(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	tournament, err := repository.GetTournamentByID(id)
	if err != nil {
		common.RespondError(c, http.StatusNotFound, "tournament not found")
		return
	}
	common.RespondProto(c, http.StatusOK, common.TournamentToProto(*tournament))
}

func handleCreateTournament(c *gin.Context) {
	var req pb.TournamentRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Name == "" {
		common.RespondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := repository.CreateTournament(req.Name, req.Slug)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusCreated, common.TournamentToProto(*tournament))
}

func handleUpdateTournament(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.TournamentRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Name == "" {
		common.RespondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := repository.UpdateTournament(id, req.Name, req.Slug)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, common.TournamentToProto(*tournament))
}

func handleDeleteTournament(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := repository.DeleteTournament(id); err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament deleted"})
}
