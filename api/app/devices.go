package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type DeviceRegistrationController struct {
	Group *gin.RouterGroup
}

func (c *DeviceRegistrationController) LoadRoutes() {
	c.Group.POST("/devices", handleRegisterDevice)
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
