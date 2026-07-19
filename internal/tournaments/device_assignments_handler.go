package tournaments

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type DeviceAssignmentsHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type DeviceAssignmentsHandler struct {
	repo     *DeviceAssignmentsRepository
	onChange func(context.Context) error
}

func NewDeviceAssignmentsHandler(repo *DeviceAssignmentsRepository) *DeviceAssignmentsHandler {
	return &DeviceAssignmentsHandler{repo: repo}
}

func (h *DeviceAssignmentsHandler) SetOnChange(fn func(context.Context) error) {
	h.onChange = fn
}

func (h *DeviceAssignmentsHandler) RegisterRoutes(group *gin.RouterGroup, deps DeviceAssignmentsHandlerDeps) {
	group.GET("/device-tournaments", deps.AuthMiddleware, h.handleGetAll)
	group.GET("/device-tournaments/page", deps.AuthMiddleware, h.handleGetPage)
	group.GET("/device-tournaments/:deviceId", deps.AuthMiddleware, h.handleGetByDevice)
	group.POST("/device-tournaments", deps.AuthMiddleware, h.handleAssign)
	group.DELETE("/device-tournaments", deps.AuthMiddleware, h.handleRemove)
	group.PUT("/device-tournaments/:deviceId", deps.AuthMiddleware, h.handleSet)
}

const dtDefaultPageLimit = 20

func (h *DeviceAssignmentsHandler) handleGetPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := dtDefaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var deviceID, tournamentID, id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 3)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		parsedDeviceID, err := server.ParseID(keys[0])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad device_id")
			return
		}
		deviceID = parsedDeviceID
		parsedTournamentID, err := server.ParseID(keys[1])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad tournament_id")
			return
		}
		tournamentID = parsedTournamentID
		parsedID, err := server.ParseID(keys[2])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	deviceTournaments, hasMore, err := h.repo.ListPage(c.Request.Context(), deviceID, tournamentID, id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(deviceTournaments) > 0 {
		last := deviceTournaments[len(deviceTournaments)-1]
		nextCursor, err = pagination.Encode(
			strconv.FormatUint(uint64(last.DeviceID), 10),
			strconv.FormatUint(uint64(last.TournamentID), 10),
			strconv.FormatUint(uint64(last.ID), 10),
		)
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.DeviceTournamentPage{
		Data: DeviceTournamentsToProto(deviceTournaments),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
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
	if h.onChange != nil {
		_ = h.onChange(c.Request.Context())
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
	if h.onChange != nil {
		_ = h.onChange(c.Request.Context())
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
	if h.onChange != nil {
		_ = h.onChange(c.Request.Context())
	}
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "device tournaments updated"})
}
