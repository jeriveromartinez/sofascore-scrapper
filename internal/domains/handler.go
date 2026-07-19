package domains

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
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
	group.GET("/domains/:id", deps.AuthMiddleware, h.handleGetDomain)
	group.POST("/domains", deps.AuthMiddleware, h.handleCreateDomain)
	group.PUT("/domains/:id", deps.AuthMiddleware, h.handleUpdateDomain)
	group.DELETE("/domains/:id", deps.AuthMiddleware, h.handleDeleteDomain)
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
