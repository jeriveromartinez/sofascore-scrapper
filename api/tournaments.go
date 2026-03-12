package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, tournaments)
}

func handleGetTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	tournament, err := repository.GetTournamentByID(id)
	if err != nil {
		respondCBOR(c, http.StatusNotFound, map[string]string{"error": "tournament not found"})
		return
	}
	respondCBOR(c, http.StatusOK, tournament)
}

func handleCreateTournament(c *gin.Context) {
	var req struct {
		Name string `json:"name" cbor:"name"`
		Slug string `json:"slug" cbor:"slug"`
	}
	if err := parseCBORBody(c, &req); err != nil || req.Name == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	tournament, err := repository.CreateTournament(req.Name, req.Slug)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusCreated, tournament)
}

func handleUpdateTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var req struct {
		Name string `json:"name" cbor:"name"`
		Slug string `json:"slug" cbor:"slug"`
	}
	if err := parseCBORBody(c, &req); err != nil || req.Name == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	tournament, err := repository.UpdateTournament(id, req.Name, req.Slug)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, tournament)
}

func handleDeleteTournament(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	if err := repository.DeleteTournament(id); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]string{"message": "tournament deleted"})
}
