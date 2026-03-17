package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/models/dto"
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
	var req dto.DeviceRegisterRequest
	if err := parseCBORBody(c, &req); err != nil || req.Token == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	device, err := repository.RegisterDevice(nil, req.Token, req.Platform, req.Name)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, device)
}

func handleGetDevices(c *gin.Context) {
	page := 1
	limit := 10
	if pageParam := c.Query("page"); pageParam != "" {
		parsedPage, parseErr := strconv.Atoi(pageParam)
		if parseErr != nil || parsedPage < 1 {
			respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "page must be a positive integer"})
			return
		}
		page = parsedPage
	}

	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, parseErr := strconv.Atoi(limitParam)
		if parseErr != nil || parsedLimit < 1 {
			respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	devices, total, err := repository.GetDevices(uint(page), uint(limit))
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	respondCBOR(c, http.StatusOK, map[string]any{
		"data":        devices,
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": totalPages,
	})
}

func handleGetAllDevices(c *gin.Context) {
	devices, err := repository.GetAllDevices()
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, map[string]any{"data": devices})
}
