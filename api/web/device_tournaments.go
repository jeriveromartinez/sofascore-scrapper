package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceTournamentController struct {
	Group *gin.RouterGroup
}

func (c *DeviceTournamentController) LoadRoutes() {
	c.Group.GET("/device-tournaments", common.AuthMiddleware(), handleGetAllDeviceTournaments)
	c.Group.GET("/device-tournaments/:deviceId", common.AuthMiddleware(), handleGetDeviceTournaments)
	c.Group.POST("/device-tournaments", common.AuthMiddleware(), handleAssignTournamentToDevice)
	c.Group.DELETE("/device-tournaments", common.AuthMiddleware(), handleRemoveTournamentFromDevice)
	c.Group.PUT("/device-tournaments/:deviceId", common.AuthMiddleware(), handleSetDeviceTournaments)
}

func handleGetAllDeviceTournaments(c *gin.Context) {
	deviceTournaments, err := repository.GetAllDeviceTournaments()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: common.DeviceTournamentsToProto(deviceTournaments)})
}

func handleGetDeviceTournaments(c *gin.Context) {
	deviceID, err := common.ParseID(c.Param("deviceId"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	deviceTournaments, err := repository.GetDeviceTournaments(deviceID)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: common.DeviceTournamentsToProto(deviceTournaments)})
}

func handleAssignTournamentToDevice(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := common.ParseProtoBody(c, &req); err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	deviceTournament, err := repository.AssignTournamentToDevice(uint(req.DeviceId), uint(req.TournamentId))
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusCreated, common.DeviceTournamentToProto(*deviceTournament))
}

func handleRemoveTournamentFromDevice(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := common.ParseProtoBody(c, &req); err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	if err := repository.RemoveTournamentFromDevice(uint(req.DeviceId), uint(req.TournamentId)); err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from device"})
}

func handleSetDeviceTournaments(c *gin.Context) {
	deviceID, err := common.ParseID(c.Param("deviceId"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	var req pb.SetTournamentIdsRequest
	if err := common.ParseProtoBody(c, &req); err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	if err := repository.SetDeviceTournaments(deviceID, ids); err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "device tournaments updated"})
}
