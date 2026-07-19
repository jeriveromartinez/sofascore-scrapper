package playback

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type PlaybackAppHandlerDeps struct {
	AppMiddleware gin.HandlerFunc
}

type AppHandler struct {
	repo    *Repository
	devRepo *devices.Repository
}

func NewAppHandler(repo *Repository, devRepo *devices.Repository) *AppHandler {
	return &AppHandler{repo: repo, devRepo: devRepo}
}

func (h *AppHandler) RegisterRoutes(group *gin.RouterGroup, deps PlaybackAppHandlerDeps) {
	group.POST("/devices/viewing", deps.AppMiddleware, h.handleReportViewing)
}

func (h *AppHandler) handleReportViewing(c *gin.Context) {
	var req pb.LogPlaybackRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.DeviceToken == "" || req.Content == "" {
		server.RespondError(c, http.StatusBadRequest, "device_token and content are required")
		return
	}

	device, err := h.devRepo.FindByToken(req.DeviceToken)
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "device not found")
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}

	playbackLog, err := h.repo.Log(device.ID, req.Content, startedAt)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.devRepo.UpdateLastSeen(req.DeviceToken); err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusCreated, PlaybackToProto(playbackLog))
}
