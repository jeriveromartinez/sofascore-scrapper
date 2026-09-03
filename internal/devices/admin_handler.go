package devices

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/pagination"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

type AdminHandlerDeps struct {
	AuthMiddleware gin.HandlerFunc
}

type AdminHandler struct {
	repo *Repository
}

func NewAdminHandler(repo *Repository) *AdminHandler {
	return &AdminHandler{repo: repo}
}

func (h *AdminHandler) RegisterRoutes(group *gin.RouterGroup, deps AdminHandlerDeps) {
	group.GET("/devices", deps.AuthMiddleware, h.handleGetDevices)
	group.GET("/devices/page", deps.AuthMiddleware, h.handleGetDevicesPage)
	group.GET("/devices/all", deps.AuthMiddleware, h.handleGetAllDevices)
	group.PUT("/devices", deps.AuthMiddleware, h.handleUpdateDevice)
}

const defaultPageLimit = 20

func (h *AdminHandler) handleGetDevicesPage(c *gin.Context) {
	cursorRaw := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := defaultPageLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var id uint
	if cursorRaw != "" {
		keys, err := pagination.Decode(cursorRaw, 1)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		parsedID, err := server.ParseID(keys[0])
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "invalid cursor: bad id")
			return
		}
		id = parsedID
	}

	devices, hasMore, err := h.repo.ListPage(c.Request.Context(), id, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var nextCursor string
	if hasMore && len(devices) > 0 {
		last := devices[len(devices)-1]
		nextCursor, err = pagination.Encode(strconv.FormatUint(uint64(last.ID), 10))
		if err != nil {
			server.RespondError(c, http.StatusInternalServerError, "cursor encoding failed")
			return
		}
	}

	server.RespondProto(c, http.StatusOK, &pb.DevicePage{
		Data: DevicesToProto(devices),
		Page: &pb.CursorPageInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	})
}

func (h *AdminHandler) handleGetDevices(c *gin.Context) {
	page := 1
	limit := 10
	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			server.RespondError(c, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			server.RespondError(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	devices, total, err := h.repo.GetDevices(uint(page), uint(limit))
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	server.RespondProto(c, http.StatusOK, &pb.DeviceList{
		Data:       DevicesToProto(devices),
		Page:       int32(page),
		Limit:      int32(limit),
		Total:      total,
		TotalPages: int32(totalPages),
	})
}

func (h *AdminHandler) handleGetAllDevices(c *gin.Context) {
	devices, err := h.repo.GetAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.DeviceList{Data: DevicesToProto(devices)})
}

func (h *AdminHandler) handleUpdateDevice(c *gin.Context) {
	var req pb.DeviceRegisterRequest
	if err := server.ParseProtoBody(c, &req); err != nil || req.Name == "" || req.PackageId == "" {
		server.RespondError(c, http.StatusBadRequest, "name and package_id are required")
		return
	}

	if req.Token == "" {
		server.RespondError(c, http.StatusBadRequest, "device token is required")
		return
	}

	updatedDevice, err := h.repo.Update(req.Token, req.Platform, req.Name, req.PackageId, req.Timezone)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, DeviceToProto(*updatedDevice))
}
