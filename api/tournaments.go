package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type TournamentController struct {
	Group *gin.RouterGroup
}

func (c *TournamentController) LoadRoutes() {
	c.Group.GET("/tournaments", authMiddleware(), handleGetTournaments)
	c.Group.GET("/tournaments/:id", authMiddleware(), handleGetTournament)
	c.Group.POST("/tournaments", authMiddleware(), handleCreateTournament)
	c.Group.PUT("/tournaments/:id", authMiddleware(), handleUpdateTournament)
	c.Group.DELETE("/tournaments/:id", authMiddleware(), handleDeleteTournament)
}

func handleGetTournaments(c *gin.Context) {
	tournaments, err := repository.GetAllTournaments()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.TournamentList{Tournaments: tournamentsToProto(tournaments)})
}

func handleGetTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	tournament, err := repository.GetTournamentByID(id)
	if err != nil {
		respondError(c, http.StatusNotFound, "tournament not found")
		return
	}
	respondProto(c, http.StatusOK, tournamentToProto(*tournament))
}

func handleCreateTournament(c *gin.Context) {
	var req pb.TournamentRequest
	if err := parseProtoBody(c, &req); err != nil || req.Name == "" {
		respondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := repository.CreateTournament(req.Name, req.Slug)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusCreated, tournamentToProto(*tournament))
}

func handleUpdateTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.TournamentRequest
	if err := parseProtoBody(c, &req); err != nil || req.Name == "" {
		respondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := repository.UpdateTournament(id, req.Name, req.Slug)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, tournamentToProto(*tournament))
}

func handleDeleteTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := repository.DeleteTournament(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament deleted"})
}
