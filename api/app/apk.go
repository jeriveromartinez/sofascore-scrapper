package app

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type ApkController struct {
	Group *gin.RouterGroup
}

func (c *ApkController) LoadRoutes() {
	c.Group.GET("/update", handleCheckApkUpdate)
	c.Group.GET("/apk/download/:token", handleDownloadApk)
}

func handleCheckApkUpdate(c *gin.Context) {
	clientVersion := c.Query("version")
	clientPackage := c.Query("package")
	if clientVersion == "" {
		common.RespondError(c, http.StatusBadRequest, "version query parameter is required")
		return
	}

	if clientPackage == "" {
		common.RespondError(c, http.StatusBadRequest, "package query parameter is required")
		return
	}

	latest, err := repository.GetLatestApkVersion(clientPackage)
	if err != nil {
		common.RespondError(c, http.StatusNotFound, "no APK version available")
		return
	}

	newer, err := repository.IsNewerVersion(clientVersion, latest.Version)
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp := &pb.ApkUpdateCheckResponse{
		UpdateAvailable: newer,
		LatestVersion:   latest.Version,
		PackageName:     latest.PackageName,
		VersionCode:     latest.VersionCode,
	}
	if newer {
		resp.DownloadUrl = "/api/app/v1/apk/download/" + latest.DownloadToken
		resp.Description = latest.Description
		resp.FileSize = latest.FileSize
		resp.MinSdkVersion = latest.MinSDKVersion
		resp.TargetSdkVersion = latest.TargetSDKVersion
	}

	common.RespondProto(c, http.StatusOK, resp)
}

func handleDownloadApk(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid download token")
		return
	}

	apk, err := repository.GetApkVersionByToken(token)
	if err != nil {
		common.RespondError(c, http.StatusNotFound, "APK version not found")
		return
	}

	storagePath, _ := filepath.Abs(common.ApkStoragePath())
	absPath, err := filepath.Abs(apk.FilePath)
	if err != nil || !strings.HasPrefix(absPath, storagePath+string(filepath.Separator)) {
		common.RespondError(c, http.StatusForbidden, "file path is invalid")
		return
	}

	_ = repository.UpdateDownloadCount(apk.ID)
	c.FileAttachment(absPath, apk.FileName)
}
