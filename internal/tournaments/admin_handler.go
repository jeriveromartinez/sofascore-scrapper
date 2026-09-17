package tournaments

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/pagination"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
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

// slugRE is the canonical slug pattern the operator-typed slug
// must match. The same shape is what ToTournament derives from
// the scraper's league name (lower-case + spaces -> dashes), so
// an admin-typed slug is round-trippable through the scraper
// path: a later upsert from the scraper will not silently
// overwrite the operator's slug because the two forms agree.
var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,190}$`)

// slugInvalidMsg is the validation error returned for malformed
// slugs. Kept as a constant so the create + update paths share
// one user-visible message.
const slugInvalidMsg = "slug must match ^[a-z0-9][a-z0-9-]{0,190}$ (lowercase letters, digits and dashes; must start with a letter or digit)"

// normalizeSlug lower-cases + trims and validates the slug. Empty
// is rejected: scraper.ToTournament always derives a non-empty
// slug from the league name, so an empty admin-typed slug would
// silently lose its identity on the next scraper tick.
func normalizeSlug(raw string) (string, bool) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if !slugRE.MatchString(slug) {
		return "", false
	}
	return slug, true
}

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

	slug, ok := normalizeSlug(req.Slug)
	if !ok {
		server.RespondError(c, http.StatusBadRequest, slugInvalidMsg)
		return
	}

	tournament, err := h.repo.Create(req.Name, slug)
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

	slug, ok := normalizeSlug(req.Slug)
	if !ok {
		server.RespondError(c, http.StatusBadRequest, slugInvalidMsg)
		return
	}

	tournament, err := h.repo.Update(id, req.Name, slug)
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
