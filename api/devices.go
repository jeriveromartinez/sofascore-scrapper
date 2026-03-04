package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceController struct {
	Group *gin.RouterGroup
}

func (c *DeviceController) LoadRoutes() {
	c.Group.POST("/devices", authMiddleware(), handleRegisterDevice)
}

func handleRegisterDevice(c *gin.Context) {
	userID := getUserID(c)
	var req struct {
		Token    string `json:"token" cbor:"token"`
		Platform string `json:"platform" cbor:"platform"`
		Name     string `json:"name" cbor:"name"`
	}
	if err := bindBody(c, &req); err != nil || req.Token == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	device, err := repository.RegisterDevice(userID, req.Token, req.Platform, req.Name)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondCBOR(c, http.StatusOK, device)
}
