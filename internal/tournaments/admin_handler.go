package tournaments

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
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
	group.GET("/tournaments/page", deps.AuthMiddleware, h.handleGetTournamentsPage)
	group.GET("/tournaments/:id", deps.AuthMiddleware, h.handleGetTournament)
	group.POST("/tournaments", deps.AuthMiddleware, h.handleCreateTournament)
	group.PUT("/tournaments/:id", deps.AuthMiddleware, h.handleUpdateTournament)
	group.DELETE("/tournaments/:id", deps.AuthMiddleware, h.handleDeleteTournament)
}

const defaultPageLimit = 20

func (h *Handler) handleGetTournamentsPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var slug string
	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 2)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		slug = keys[0]
		parsedID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	tournaments, hasMore, err := h.repo.ListPage(c.Request.Context(), slug, id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(tournaments) > 0 {
		last := tournaments[len(tournaments)-1]
		nextCursor, err = pagination.Encode(last.Slug, strconv.FormatUint(uint64(last.ID), 10))
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.TournamentPage{
		Data: TournamentsToProto(tournaments),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
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
