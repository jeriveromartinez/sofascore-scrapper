package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type PlaybackController struct {
	Group *gin.RouterGroup
}

func (c *PlaybackController) LoadRoutes() {
	c.Group.POST("/playback", authMiddleware(), handleLogPlayback)
	c.Group.PUT("/playback/:id", authMiddleware(), handleUpdatePlayback)
	c.Group.PATCH("/playback/:id", authMiddleware(), handleUpdatePlayback)
}

func handleLogPlayback(c *gin.Context) {
	var req pb.LogPlaybackRequest
	if err := parseProtoBody(c, &req); err != nil || req.SofaScoreEventId == 0 {
		respondError(c, http.StatusBadRequest, "sofa_score_event_id is required")
		return
	}

	db, err := database.GetDB()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var device models.Device
	if err := db.Where("token = ?", req.DeviceToken).First(&device).Error; err != nil {
		respondError(c, http.StatusBadRequest, "device not found")
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}
	playbackLog, err := repository.LogPlayback(device.ID, req.SofaScoreEventId, startedAt)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusCreated, playbackToProto(playbackLog))
}

func handleUpdatePlayback(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req pb.UpdatePlaybackRequest
	if err := parseProtoBody(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid body")
		return
	}
	endedAt := req.EndedAt
	if endedAt == 0 {
		endedAt = time.Now().Unix()
	}
	if err := repository.UpdatePlaybackEnd(uint(id), endedAt); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.StatusResponse{Status: "updated"})
}
