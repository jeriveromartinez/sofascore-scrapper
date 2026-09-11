// internal/scraper/catalog/handler.go
package catalog

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	g := r.Group("/scraper-leagues")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/search", h.Search)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}

// createLeagueRequest is the wire DTO for POST /scraper-leagues. We use a
// separate struct (rather than binding directly into ScraperLeague) because
// the model uses camelCase Go field names without json tags, while the API
// contract exposes snake_case (e.g. source_league_id).
type createLeagueRequest struct {
	Source         string `json:"source"`
	SourceLeagueId string `json:"source_league_id"`
	Name           string `json:"name"`
	Country        string `json:"country"`
	Sport          string `json:"sport"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createLeagueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": err.Error()})
		return
	}
	sl := ScraperLeague{
		Source:         req.Source,
		SourceLeagueId: req.SourceLeagueId,
		Name:           req.Name,
		Country:        req.Country,
		Sport:          req.Sport,
	}
	if req.Enabled != nil {
		sl.Enabled = *req.Enabled
	}
	if err := h.svc.Create(c.Request.Context(), &sl); err != nil {
		if errors.Is(err, ErrDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate", "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation", "message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, sl)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	sl, err := h.svc.Get(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
		return
	}
	if sl == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, sl)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	var fields map[string]any
	if err := c.ShouldBindJSON(&fields); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	// Filter allowed fields
	allowed := map[string]bool{"name": true, "country": true, "sport": true, "enabled": true}
	safe := map[string]any{}
	for k, v := range fields {
		if allowed[k] {
			safe[k] = v
		}
	}
	if len(safe) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_editable_fields"})
		return
	}
	if err := h.svc.Update(c.Request.Context(), uint(id), safe); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) List(c *gin.Context) {
	f := ListFilters{
		Source:  c.Query("source"),
		Country: c.Query("country"),
		Query:   c.Query("q"),
	}
	if e := c.Query("enabled"); e != "" {
		b := e == "true" || e == "1"
		f.Enabled = &b
	}
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		f.Page = p
	}
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		f.Limit = l
	}
	items, total, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":  items,
		"total": total,
		"page":  f.Page,
		"limit": f.Limit,
	})
}

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_query"})
		return
	}
	results, err := h.svc.SearchLeagues(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}
