package tournaments

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type HandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) RegisterRoutes(group *gin.RouterGroup, deps HandlerDeps) {
	group.GET("/tournaments", deps.AuthMiddleware, h.handleGetTournaments)
	group.GET("/tournaments/:id", deps.AuthMiddleware, h.handleGetTournament)
	group.POST("/tournaments", deps.AuthMiddleware, h.handleCreateTournament)
	group.PUT("/tournaments/:id", deps.AuthMiddleware, h.handleUpdateTournament)
	group.DELETE("/tournaments/:id", deps.AuthMiddleware, h.handleDeleteTournament)
}

func (h *Handler) handleGetTournaments(c *gin.Context) {
	tournaments, err := h.repo.GetAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.TournamentList{Tournaments: TournamentsToProto(tournaments)})
}

func (h *Handler) handleGetTournament(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	tournament, err := h.repo.GetByID(id)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "tournament not found")
		return
	}
	server.RespondProto(c, http.StatusOK, TournamentToProto(*tournament))
}

func (h *Handler) handleCreateTournament(c *gin.Context) {
	var req pb.TournamentRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Name == "" {
		server.RespondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := h.repo.Create(req.Name, req.Slug)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusCreated, TournamentToProto(*tournament))
}

func (h *Handler) handleUpdateTournament(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.TournamentRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Name == "" {
		server.RespondError(c, http.StatusBadRequest, "name is required")
		return
	}

	tournament, err := h.repo.Update(id, req.Name, req.Slug)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, TournamentToProto(*tournament))
}

func (h *Handler) handleDeleteTournament(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament deleted"})
}
