package devices

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type AppHandler struct {
	repo *Repository
}

func NewAppHandler(repo *Repository) *AppHandler {
	return &AppHandler{repo: repo}
}

func (h *AppHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/devices", h.handleRegisterDevice)
}

func (h *AppHandler) handleRegisterDevice(c *gin.Context) {
	var req pb.DeviceRegisterRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Token == "" {
		server.RespondError(c, http.StatusBadRequest, "token is required")
		return
	}

	var domainID *uint
	if req.DomainId != 0 {
		id := uint(req.DomainId)
		domainID = &id
	}

	device, err := h.repo.Register(nil, domainID, req.Token, req.Platform, req.Name, req.Version)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, DeviceToProto(*device))
}
