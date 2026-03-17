package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
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
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: deviceTournamentsToProto(deviceTournaments)})
}

func handleGetDeviceTournaments(c *gin.Context) {
	deviceID, err := parseID(c.Param("deviceId"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	deviceTournaments, err := repository.GetDeviceTournaments(deviceID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: deviceTournamentsToProto(deviceTournaments)})
}

func handleAssignTournamentToDevice(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := parseProtoBody(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	deviceTournament, err := repository.AssignTournamentToDevice(uint(req.DeviceId), uint(req.TournamentId))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusCreated, deviceTournamentToProto(*deviceTournament))
}

func handleRemoveTournamentFromDevice(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := parseProtoBody(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	if err := repository.RemoveTournamentFromDevice(uint(req.DeviceId), uint(req.TournamentId)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from device"})
}

func handleSetDeviceTournaments(c *gin.Context) {
	deviceID, err := parseID(c.Param("deviceId"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	var req pb.SetTournamentIdsRequest
	if err := parseProtoBody(c, &req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	if err := repository.SetDeviceTournaments(deviceID, ids); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.StatusMessage{Message: "device tournaments updated"})
}
