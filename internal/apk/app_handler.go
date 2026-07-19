package apk

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

type AppHandler struct {
	repo *Repository
}

func NewAppHandler(repo *Repository) *AppHandler {
	return &AppHandler{repo: repo}
}

func (h *AppHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/update", h.handleCheckUpdate)
	group.GET("/apk/download/:token", h.handleDownload)
	group.GET("/devices/url/:packageName", h.handleGetDomain)
}

func (h *AppHandler) handleCheckUpdate(c *gin.Context) {
	clientVersion := c.Query("version")
	clientPackage := c.Query("package")
	if clientVersion == "" {
		server.RespondError(c, http.StatusBadRequest, "version query parameter is required")
		return
	}

	if clientPackage == "" {
		server.RespondError(c, http.StatusBadRequest, "package query parameter is required")
		return
	}

	latest, err := h.repo.GetLatest(clientPackage)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "no APK version available")
		return
	}

	newer, err := IsNewerVersion(clientVersion, latest.Version)
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, err.Error())
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

	server.RespondProto(c, http.StatusOK, resp)
}

func (h *AppHandler) handleDownload(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid download token")
		return
	}

	apk, err := h.repo.GetByToken(token)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "APK version not found")
		return
	}

	storagePath, _ := filepath.Abs(StoragePath())
	absPath, err := filepath.Abs(apk.FilePath)
	if err != nil || !strings.HasPrefix(absPath, storagePath+string(filepath.Separator)) {
		server.RespondError(c, http.StatusForbidden, "file path is invalid")
		return
	}

	if rc := http.NewResponseController(c.Writer); rc != nil {
		rc.SetWriteDeadline(time.Time{})
	}

	_ = h.repo.IncrementDownloadCount(apk.ID)
	c.FileAttachment(absPath, apk.FileName)
}

func (h *AppHandler) handleGetDomain(c *gin.Context) {
	packageName := c.Param("packageName")
	if packageName == "" {
		server.RespondError(c, http.StatusBadRequest, "packageName is required")
		return
	}

	parts := len(strings.Split(packageName, "."))
	if parts < 3 || parts > 4 {
		server.RespondError(c, http.StatusBadRequest, "invalid packageName format")
		return
	}

	apk, err := h.repo.GetLatest(packageName)
	if err != nil {
		server.RespondError(c, http.StatusNotFound, "APK version not found")
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.DeviceUrl{Url: apk.IPTVUrl})
}
