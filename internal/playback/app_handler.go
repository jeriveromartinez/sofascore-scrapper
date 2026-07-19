package playback

import (
	"context"
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
	service *Service
}

func NewAppHandler(service *Service) *AppHandler {
	return &AppHandler{service: service}
}

func (h *AppHandler) RegisterRoutes(group *gin.RouterGroup, deps PlaybackAppHandlerDeps) {
	group.POST("/devices/viewing", deps.AppMiddleware, h.handleReportViewing)
}

func (h *AppHandler) handleReportViewing(c *gin.Context) {
	var req pb.LogPlaybackRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Content == "" {
		server.RespondError(c, http.StatusBadRequest, "content is required")
		return
	}

	dev, ok := c.Get("device")
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "device not authenticated")
		return
	}
	device := dev.(devices.Device)

	if req.DeviceToken != "" && req.DeviceToken != device.Token {
		server.RespondError(c, http.StatusForbidden, "device token mismatch")
		return
	}

	startedAt, err := NormalizeUnixMillis(req.StartedAt, time.Now)
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	playbackLog, err := h.service.Start(context.Background(), device, req.Content, startedAt)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusCreated, PlaybackToProto(playbackLog))
}
