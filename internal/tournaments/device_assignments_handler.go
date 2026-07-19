package tournaments

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type DeviceAssignmentsHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type DeviceAssignmentsHandler struct {
	repo *DeviceAssignmentsRepository
}

func NewDeviceAssignmentsHandler(repo *DeviceAssignmentsRepository) *DeviceAssignmentsHandler {
	return &DeviceAssignmentsHandler{repo: repo}
}

func (h *DeviceAssignmentsHandler) RegisterRoutes(group *gin.RouterGroup, deps DeviceAssignmentsHandlerDeps) {
	group.GET("/device-tournaments", deps.AuthMiddleware, h.handleGetAll)
	group.GET("/device-tournaments/:deviceId", deps.AuthMiddleware, h.handleGetByDevice)
	group.POST("/device-tournaments", deps.AuthMiddleware, h.handleAssign)
	group.DELETE("/device-tournaments", deps.AuthMiddleware, h.handleRemove)
	group.PUT("/device-tournaments/:deviceId", deps.AuthMiddleware, h.handleSet)
}

func (h *DeviceAssignmentsHandler) handleGetAll(c *gin.Context) {
	deviceTournaments, err := h.repo.GetAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: DeviceTournamentsToProto(deviceTournaments)})
}

func (h *DeviceAssignmentsHandler) handleGetByDevice(c *gin.Context) {
	deviceID, err := server.ParseID(c.Param("deviceId"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	deviceTournaments, err := h.repo.GetByDevice(deviceID)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.DeviceTournamentList{DeviceTournaments: DeviceTournamentsToProto(deviceTournaments)})
}

func (h *DeviceAssignmentsHandler) handleAssign(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	deviceTournament, err := h.repo.Assign(uint(req.DeviceId), uint(req.TournamentId))
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusCreated, DeviceTournamentToProto(*deviceTournament))
}

func (h *DeviceAssignmentsHandler) handleRemove(c *gin.Context) {
	var req pb.AssignTournamentRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.repo.Remove(uint(req.DeviceId), uint(req.TournamentId)); err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "tournament removed from device"})
}

func (h *DeviceAssignmentsHandler) handleSet(c *gin.Context) {
	deviceID, err := server.ParseID(c.Param("deviceId"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid device id")
		return
	}

	var req pb.SetTournamentIdsRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	ids := make([]uint, len(req.TournamentIds))
	for i, id := range req.TournamentIds {
		ids[i] = uint(id)
	}

	if err := h.repo.Set(deviceID, ids); err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "device tournaments updated"})
}
