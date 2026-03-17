package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceController struct {
	Group *gin.RouterGroup
}

func (c *DeviceController) LoadRoutes() {
	c.Group.GET("/devices", authMiddleware(), handleGetDevices)
	c.Group.GET("/devices/all", authMiddleware(), handleGetAllDevices)
	c.Group.POST("/devices", handleRegisterDevice)
}

func handleRegisterDevice(c *gin.Context) {
	var req pb.DeviceRegisterRequest
	if err := parseProtoBody(c, &req); err != nil || req.Token == "" {
		respondError(c, http.StatusBadRequest, "token is required")
		return
	}
	device, err := repository.RegisterDevice(nil, req.Token, req.Platform, req.Name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, deviceToProto(*device))
}

func handleGetDevices(c *gin.Context) {
	page := 1
	limit := 10
	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			respondError(c, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			respondError(c, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	devices, total, err := repository.GetDevices(uint(page), uint(limit))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	respondProto(c, http.StatusOK, &pb.DeviceList{
		Data:       devicesToProto(devices),
		Page:       int32(page),
		Limit:      int32(limit),
		Total:      total,
		TotalPages: int32(totalPages),
	})
}

func handleGetAllDevices(c *gin.Context) {
	devices, err := repository.GetAllDevices()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondProto(c, http.StatusOK, &pb.DeviceList{Data: devicesToProto(devices)})
}
