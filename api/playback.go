package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type PlaybackController struct {
	Group *gin.RouterGroup
}

func (c *PlaybackController) LoadRoutes() {
	auth := authMiddleware()
	c.Group.POST("/playback", auth, handleLogPlayback)
	c.Group.PUT("/playback/:id", auth, handleUpdatePlayback)
	c.Group.PATCH("/playback/:id", auth, handleUpdatePlayback)
}

func handleLogPlayback(c *gin.Context) {
	var req struct {
		DeviceToken      string `json:"device_token" cbor:"device_token"`
		SofaScoreEventId int64  `json:"sofa_score_event_id" cbor:"sofa_score_event_id"`
		StartedAt        int64  `json:"started_at" cbor:"started_at"`
	}
	if err := parseCBORBody(c, &req); err != nil || req.SofaScoreEventId == 0 {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "sofa_score_event_id is required"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var device models.Device
	if err := db.Where("token = ?", req.DeviceToken).First(&device).Error; err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "device not found"})
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}
	playbackLog, err := repository.LogPlayback(device.ID, req.SofaScoreEventId, startedAt)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusCreated, playbackLog)
}

func handleUpdatePlayback(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		EndedAt int64 `json:"ended_at" cbor:"ended_at"`
	}
	if err := parseCBORBody(c, &req); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	endedAt := req.EndedAt
	if endedAt == 0 {
		endedAt = time.Now().Unix()
	}
	if err := repository.UpdatePlaybackEnd(uint(id), endedAt); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]string{"status": "updated"})
}
