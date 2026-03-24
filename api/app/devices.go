package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceRegistrationController struct {
	Group *gin.RouterGroup
}

func (c *DeviceRegistrationController) LoadRoutes() {
	c.Group.POST("/devices", handleRegisterDevice)
	c.Group.POST("/devices/viewing", common.AppMiddleware(), handleReportViewing)
}

func handleRegisterDevice(c *gin.Context) {
	var req pb.DeviceRegisterRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Token == "" {
		common.RespondError(c, http.StatusBadRequest, "token is required")
		return
	}
	device, err := repository.RegisterDevice(nil, req.Token, req.Platform, req.Name)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.RespondProto(c, http.StatusOK, common.DeviceToProto(*device))
}

func handleReportViewing(c *gin.Context) {
	var req pb.LogPlaybackRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.DeviceToken == "" || req.Content == "" {
		common.RespondError(c, http.StatusBadRequest, "device_token and content are required")
		return
	}

	db, err := database.GetDB()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var device models.Device
	if err := db.Where("token = ?", req.DeviceToken).First(&device).Error; err != nil {
		common.RespondError(c, http.StatusBadRequest, "device not found")
		return
	}

	startedAt := req.StartedAt
	if startedAt == 0 {
		startedAt = time.Now().Unix()
	}

	playbackLog, err := repository.LogPlayback(device.ID, req.Content, startedAt)
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := repository.UpdateDeviceLastSeen(req.DeviceToken); err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusCreated, common.PlaybackToProto(playbackLog))
}
