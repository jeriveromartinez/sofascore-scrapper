package devices

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
	"gorm.io/gorm"
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
	if err := server.ParseProtoBody(c, &req); err != nil || req.Token == "" || req.PackageId == "" {
		server.RespondError(c, http.StatusBadRequest, "token and package_id are required")
		return
	}

	device, err := h.repo.Register(c.Request.Context(), nil, uint(req.DomainId), req.Token, req.Platform, req.Name, req.Version, req.PackageId, req.Timezone)
	if errors.Is(err, gorm.ErrInvalidData) {
		server.RespondError(c, http.StatusBadRequest, "invalid device registration")
		return
	}
	if err != nil {
		respondInternalError(c, "register device failed", err)
		return
	}
	server.RespondProto(c, http.StatusOK, DeviceToProto(*device))
}
