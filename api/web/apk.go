package web

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
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	"github.com/jeriveromartinez/sofascore-scrapper/apkutil"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
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

type ApkController struct {
	Group *gin.RouterGroup
}

func (c *ApkController) LoadRoutes() {
	c.Group.POST("/apk/upload", common.AuthMiddleware(), handleUploadApk)
	c.Group.POST("/apk/upload/chunk", common.AuthMiddleware(), handleUploadChunk)
	c.Group.POST("/apk/upload/assemble", common.AuthMiddleware(), handleAssembleChunks)
	c.Group.GET("/apk/versions", common.AuthMiddleware(), handleListApkVersions)
}

func handleUploadApk(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "file is required")
		return
	}

	storagePath := common.ApkStoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		common.RespondError(c, http.StatusInternalServerError, "could not create storage directory")
		return
	}

	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		common.RespondError(c, http.StatusInternalServerError, "could not save file")
		return
	}

	apkInfo, parseErr := apkutil.ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		common.RespondError(c, http.StatusBadRequest, "could not parse APK metadata: "+parseErr.Error())
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
		common.RespondError(c, http.StatusBadRequest, msg)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		common.RespondError(c, http.StatusInternalServerError, "could not finalize file")
		return
	}

	description := c.PostForm("description")
	apk, err := repository.CreateApkVersion(
		version, fileName, destPath, description, apkInfo.PackageName,
		fileHeader.Size, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		common.RespondError(c, http.StatusConflict, "could not save APK version: "+err.Error())
		return
	}

	common.RespondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
		Id:               uint32(apk.ID),
		Version:          apk.Version,
		FileName:         apk.FileName,
		FileSize:         apk.FileSize,
		Description:      apk.Description,
		PackageName:      apk.PackageName,
		VersionCode:      apk.VersionCode,
		MinSdkVersion:    apk.MinSDKVersion,
		TargetSdkVersion: apk.TargetSDKVersion,
		DownloadToken:    apk.DownloadToken,
		DownloadUrl:      fmt.Sprintf("/api/app/v1/apk/download/%s", apk.DownloadToken),
		CreatedAt:        apk.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func handleUploadChunk(c *gin.Context) {
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

	chunkDir := filepath.Join(common.ApkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(common.ApkStoragePath())
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

func handleAssembleChunks(c *gin.Context) {
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

	chunkDir := filepath.Join(common.ApkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(common.ApkStoragePath())
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

	storagePath := common.ApkStoragePath()
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

	apkInfo, parseErr := apkutil.ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		c.JSON(http.StatusBadRequest, map[string]string{"error": "could not parse APK metadata: " + parseErr.Error()})
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
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not finalize file"})
		return
	}

	description := c.PostForm("description")
	apk, err := repository.CreateApkVersion(
		version, fileName, destPath, description, apkInfo.PackageName,
		totalSize, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		c.JSON(http.StatusConflict, map[string]string{"error": "could not save APK version: " + err.Error()})
		return
	}

	common.RespondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
		Id:               uint32(apk.ID),
		Version:          apk.Version,
		FileName:         apk.FileName,
		FileSize:         apk.FileSize,
		Description:      apk.Description,
		PackageName:      apk.PackageName,
		VersionCode:      apk.VersionCode,
		MinSdkVersion:    apk.MinSDKVersion,
		TargetSdkVersion: apk.TargetSDKVersion,
		DownloadToken:    apk.DownloadToken,
		DownloadUrl:      fmt.Sprintf("/api/app/v1/apk/download/%s", apk.DownloadToken),
		CreatedAt:        apk.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func handleListApkVersions(c *gin.Context) {
	versions, err := repository.ListApkVersions()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.ApkList{Versions: common.ApksToProto(versions)})
}
