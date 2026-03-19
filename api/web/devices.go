package web

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceController struct {
	Group *gin.RouterGroup
}

func (c *DeviceController) LoadRoutes() {
	c.Group.GET("/devices", common.AuthMiddleware(), handleGetDevices)
	c.Group.GET("/devices/all", common.AuthMiddleware(), handleGetAllDevices)
}

func handleGetDevices(c *gin.Context) {
	page := 1
	limit := 10
	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			common.RespondError(c, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			common.RespondError(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	devices, total, err := repository.GetDevices(uint(page), uint(limit))
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	common.RespondProto(c, http.StatusOK, &pb.DeviceList{
		Data:       common.DevicesToProto(devices),
		Page:       int32(page),
		Limit:      int32(limit),
		Total:      total,
		TotalPages: int32(totalPages),
	})
}

func handleGetAllDevices(c *gin.Context) {
	devices, err := repository.GetAllDevices()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, &pb.DeviceList{Data: common.DevicesToProto(devices)})
}
