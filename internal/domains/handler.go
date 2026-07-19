package domains

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"gorm.io/gorm"
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
	group.GET("/domains", deps.AuthMiddleware, h.handleGetDomains)
	group.GET("/domains/page", deps.AuthMiddleware, h.handleGetDomainsPage)
	group.GET("/domains/:id", deps.AuthMiddleware, h.handleGetDomain)
	group.POST("/domains", deps.AuthMiddleware, h.handleCreateDomain)
	group.PUT("/domains/:id", deps.AuthMiddleware, h.handleUpdateDomain)
	group.DELETE("/domains/:id", deps.AuthMiddleware, h.handleDeleteDomain)
}

const defaultPageLimit = 20

func (h *Handler) handleGetDomainsPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var domainStr string
	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 2)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		domainStr = keys[0]
		parsedID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	domains, hasMore, err := h.repo.ListPage(c.Request.Context(), domainStr, id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(domains) > 0 {
		last := domains[len(domains)-1]
		nextCursor, err = pagination.Encode(last.Domain, strconv.FormatUint(uint64(last.ID), 10))
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.DomainPage{
		Data: DomainsToProto(domains),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
}

func (h *Handler) handleGetDomains(c *gin.Context) {
	domains, err := h.repo.GetAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.DomainList{Domains: DomainsToProto(domains)})
}

func (h *Handler) handleGetDomain(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	domain, err := h.repo.GetByID(id)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "domain not found")
		return
	}

	server.RespondProto(c, http.StatusOK, DomainToProto(*domain))
}

func (h *Handler) handleCreateDomain(c *gin.Context) {
	var req pb.DomainRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Domain == "" || req.UserId == 0 {
		server.RespondError(c, http.StatusBadRequest, "domain and user_id are required")
		return
	}

	domain, err := h.repo.Create(req.Domain, uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "user not found")
			return
		}
		server.RespondError(c, http.StatusConflict, "could not create domain")
		return
	}

	server.RespondProto(c, http.StatusCreated, DomainToProto(*domain))
}

func (h *Handler) handleUpdateDomain(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.DomainRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Domain == "" || req.UserId == 0 {
		server.RespondError(c, http.StatusBadRequest, "domain and user_id are required")
		return
	}

	domain, err := h.repo.Update(id, req.Domain, uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "domain or user not found")
			return
		}
		server.RespondError(c, http.StatusConflict, "could not update domain")
		return
	}

	server.RespondProto(c, http.StatusOK, DomainToProto(*domain))
}

func (h *Handler) handleDeleteDomain(c *gin.Context) {
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondError(c, http.StatusNotFound, "domain not found")
			return
		}
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "domain deleted"})
}
