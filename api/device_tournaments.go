package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceTournamentController struct {
	Group *gin.RouterGroup
}

func (c *DeviceTournamentController) LoadRoutes() {
	c.Group.GET("/device-tournaments", authMiddleware(), handleGetAllDeviceTournaments)
	c.Group.GET("/device-tournaments/:deviceId", authMiddleware(), handleGetDeviceTournaments)
	c.Group.POST("/device-tournaments", authMiddleware(), handleAssignTournamentToDevice)
	c.Group.DELETE("/device-tournaments", authMiddleware(), handleRemoveTournamentFromDevice)
	c.Group.PUT("/device-tournaments/:deviceId", authMiddleware(), handleSetDeviceTournaments)
}

func handleGetAllDeviceTournaments(c *gin.Context) {
	deviceTournaments, err := repository.GetAllDeviceTournaments()
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, deviceTournaments)
}

func handleGetDeviceTournaments(c *gin.Context) {
	deviceID, err := parseID(c.Param("deviceId"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	deviceTournaments, err := repository.GetDeviceTournaments(deviceID)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, deviceTournaments)
}

func handleAssignTournamentToDevice(c *gin.Context) {
	var req struct {
		DeviceID     uint `json:"device_id" cbor:"device_id"`
		TournamentID uint `json:"tournament_id" cbor:"tournament_id"`
	}
	if err := parseCBORBody(c, &req); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	deviceTournament, err := repository.AssignTournamentToDevice(req.DeviceID, req.TournamentID)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusCreated, deviceTournament)
}

func handleRemoveTournamentFromDevice(c *gin.Context) {
	var req struct {
		DeviceID     uint `json:"device_id" cbor:"device_id"`
		TournamentID uint `json:"tournament_id" cbor:"tournament_id"`
	}
	if err := parseCBORBody(c, &req); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := repository.RemoveTournamentFromDevice(req.DeviceID, req.TournamentID); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]string{"message": "tournament removed from device"})
}

func handleSetDeviceTournaments(c *gin.Context) {
	deviceID, err := parseID(c.Param("deviceId"))
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	var req struct {
		TournamentIDs []uint `json:"tournament_ids" cbor:"tournament_ids"`
	}
	if err := parseCBORBody(c, &req); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := repository.SetDeviceTournaments(deviceID, req.TournamentIDs); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]string{"message": "device tournaments updated"})
}
