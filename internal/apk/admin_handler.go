package apk

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

const (
	maxChunkSize   = 20 * 1024 * 1024
	maxTotalChunks = 1000
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func randomSuffix() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

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
	group.POST("/apk/upload", deps.AuthMiddleware, h.handleUpload)
	group.POST("/apk/upload/chunk", deps.AuthMiddleware, h.handleUploadChunk)
	group.POST("/apk/upload/assemble", deps.AuthMiddleware, h.handleAssembleChunks)
	group.GET("/apk/versions", deps.AuthMiddleware, h.handleListVersions)
	group.PUT("/apk/:id", deps.AuthMiddleware, h.handleUpdateVersion)
}

func (h *AdminHandler) handleUpload(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "file is required")
		return
	}

	storagePath := StoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		server.RespondError(c, http.StatusInternalServerError, "could not create storage directory")
		return
	}

	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		server.RespondError(c, http.StatusInternalServerError, "could not save file")
		return
	}

	apkInfo, parseErr := ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		server.RespondError(c, http.StatusBadRequest, "could not parse APK metadata: "+parseErr.Error())
		return
	}

	version := c.PostForm("version")
	if version == "" {
		version = apkInfo.VersionName
	}
	if version == "" || !semverPattern.MatchString(version) {
		_ = os.Remove(tmpPath)
		msg := "version must be in MAJOR.MINOR.PATCH format; provide it via the 'version' field or ensure the APK versionName uses that format"
		if apkInfo.VersionName != "" {
			msg += " (apk_version_name: " + apkInfo.VersionName + ")"
		}
		server.RespondError(c, http.StatusBadRequest, msg)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		server.RespondError(c, http.StatusInternalServerError, "could not finalize file")
		return
	}

	description := c.PostForm("description")
	apkV, err := h.repo.Create(
		version, fileName, destPath, description, apkInfo.PackageName,
		fileHeader.Size, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		server.RespondError(c, http.StatusConflict, "could not save APK version: "+err.Error())
		return
	}

	server.RespondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
		Id:               uint32(apkV.ID),
		Version:          apkV.Version,
		FileName:         apkV.FileName,
		FileSize:         apkV.FileSize,
		Description:      apkV.Description,
		PackageName:      apkV.PackageName,
		VersionCode:      apkV.VersionCode,
		MinSdkVersion:    apkV.MinSDKVersion,
		TargetSdkVersion: apkV.TargetSDKVersion,
		DownloadToken:    apkV.DownloadToken,
		DownloadUrl:      fmt.Sprintf("/api/app/v1/apk/download/%s", apkV.DownloadToken),
		CreatedAt:        apkV.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *AdminHandler) handleUploadChunk(c *gin.Context) {
	uploadID := c.PostForm("upload_id")
	if _, err := uuid.Parse(uploadID); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	chunkIndex, err := strconv.Atoi(c.PostForm("chunk_index"))
	if err != nil || chunkIndex < 0 {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid chunk_index"})
		return
	}

	totalChunks, err := strconv.Atoi(c.PostForm("total_chunks"))
	if err != nil || totalChunks <= 0 || totalChunks > maxTotalChunks {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid total_chunks"})
		return
	}

	if chunkIndex >= totalChunks {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "chunk_index must be less than total_chunks"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "chunk data is required"})
		return
	}

	if fileHeader.Size > maxChunkSize {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "chunk size exceeds maximum allowed size"})
		return
	}

	chunkDir := filepath.Join(StoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(StoragePath())
	absChunkDir, err := filepath.Abs(chunkDir)
	if err != nil || !strings.HasPrefix(absChunkDir, absStoragePath+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create chunk directory"})
		return
	}

	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", chunkIndex))
	if err := c.SaveUploadedFile(fileHeader, chunkPath); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not save chunk"})
		return
	}

	c.JSON(http.StatusOK, map[string]any{
		"upload_id":    uploadID,
		"chunk_index":  chunkIndex,
		"total_chunks": totalChunks,
	})
}

func (h *AdminHandler) handleAssembleChunks(c *gin.Context) {
	uploadID := c.PostForm("upload_id")
	if _, err := uuid.Parse(uploadID); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	totalChunks, err := strconv.Atoi(c.PostForm("total_chunks"))
	if err != nil || totalChunks <= 0 || totalChunks > maxTotalChunks {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid total_chunks"})
		return
	}

	chunkDir := filepath.Join(StoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(StoragePath())
	absChunkDir, err := filepath.Abs(chunkDir)
	if err != nil || !strings.HasPrefix(absChunkDir, absStoragePath+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", i))
		if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
			c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("chunk %d is missing", i)})
			return
		}
	}

	storagePath := StoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create storage directory"})
		return
	}

	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	outFile, err := os.Create(tmpPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create temporary file"})
		return
	}

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			outFile.Close()
			_ = os.Remove(tmpPath)
			c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("could not read chunk %d", i)})
			return
		}
		_, copyErr := io.Copy(outFile, chunkFile)
		chunkFile.Close()
		if copyErr != nil {
			outFile.Close()
			_ = os.Remove(tmpPath)
			c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not write assembled file"})
			return
		}
	}

	totalSize := int64(0)
	if info, err := outFile.Stat(); err == nil {
		totalSize = info.Size()
	}
	outFile.Close()

	_ = os.RemoveAll(chunkDir)

	apkInfo, parseErr := ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		server.RespondError(c, http.StatusBadRequest, "could not parse APK metadata: "+parseErr.Error())
		return
	}

	version := c.PostForm("version")
	if version == "" {
		version = apkInfo.VersionName
	}
	if version == "" || !semverPattern.MatchString(version) {
		_ = os.Remove(tmpPath)
		errResp := map[string]string{
			"error": "version must be in MAJOR.MINOR.PATCH format; provide it via the 'version' field or ensure the APK versionName uses that format",
		}
		if apkInfo.VersionName != "" {
			errResp["apk_version_name"] = apkInfo.VersionName
		}
		c.JSON(http.StatusBadRequest, errResp)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		server.RespondError(c, http.StatusInternalServerError, "could not finalize file")
		return
	}

	description := c.PostForm("description")
	apkV, err := h.repo.Create(
		version, fileName, destPath, description, apkInfo.PackageName,
		totalSize, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		server.RespondError(c, http.StatusConflict, "could not save APK version: "+err.Error())
		return
	}

	server.RespondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
		Id:               uint32(apkV.ID),
		Version:          apkV.Version,
		FileName:         apkV.FileName,
		FileSize:         apkV.FileSize,
		Description:      apkV.Description,
		PackageName:      apkV.PackageName,
		VersionCode:      apkV.VersionCode,
		MinSdkVersion:    apkV.MinSDKVersion,
		TargetSdkVersion: apkV.TargetSDKVersion,
		DownloadToken:    apkV.DownloadToken,
		DownloadUrl:      fmt.Sprintf("/api/app/v1/apk/download/%s", apkV.DownloadToken),
		CreatedAt:        apkV.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *AdminHandler) handleListVersions(c *gin.Context) {
	versions, err := h.repo.ListAll()
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.ApkList{Versions: ApksToProto(versions)})
}

func (h *AdminHandler) handleUpdateVersion(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		server.RespondError(c, http.StatusBadRequest, "invalid APK ID")
		return
	}

	akpData := pb.DeviceUrl{}
	err = server.ParseProtoBody(c, &akpData)
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	err = h.repo.UpdateURL(uint(id), akpData.Url)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, "could not update APK URL: "+err.Error())
		return
	}

	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "ok"})
}
