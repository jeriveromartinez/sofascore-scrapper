package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func handleLogPlayback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceToken      string `json:"device_token" cbor:"device_token"`
		SofaScoreEventId int64  `json:"sofa_score_event_id" cbor:"sofa_score_event_id"`
		StartedAt        int64  `json:"started_at" cbor:"started_at"`
	}
	if err := decodeBody(r, &req); err != nil || req.SofaScoreEventId == 0 {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "sofa_score_event_id is required"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var device models.Device
	if err := db.Where("token = ?", req.DeviceToken).First(&device).Error; err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "device not found"})
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}
	playbackLog, err := repository.LogPlayback(device.ID, req.SofaScoreEventId, startedAt)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusCreated, playbackLog)
}

func handleUpdatePlayback(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/playback/")
	if strings.Contains(idStr, "/") || idStr == "" {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		EndedAt int64 `json:"ended_at" cbor:"ended_at"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeCBOR(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	endedAt := req.EndedAt
	if endedAt == 0 {
		endedAt = time.Now().Unix()
	}
	if err := repository.UpdatePlaybackEnd(uint(id), endedAt); err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusOK, map[string]string{"status": "updated"})
}
