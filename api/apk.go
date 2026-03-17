package api

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
	"github.com/jeriveromartinez/sofascore-scrapper/apkutil"
	pb "github.com/jeriveromartinez/sofascore-scrapper/pb"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

// maxChunkSize is the maximum allowed size of a single uploaded chunk (20 MB).
const maxChunkSize = 20 * 1024 * 1024

// maxTotalChunks is the maximum number of chunks allowed per upload session.
const maxTotalChunks = 1000

// semverPattern accepts only digits and dots, e.g. "1.2.3".
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func randomSuffix() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// ApkController handles APK upload, version check, and download.
type ApkController struct {
	Group *gin.RouterGroup
}

func (c *ApkController) LoadRoutes() {
	// Admin: upload a new APK (requires authentication)
	c.Group.POST("/apk/upload", authMiddleware(), handleUploadApk)
	// Admin: chunked upload – send one chunk at a time (requires authentication)
	c.Group.POST("/apk/upload/chunk", authMiddleware(), handleUploadChunk)
	// Admin: assemble previously uploaded chunks into a final APK (requires authentication)
	c.Group.POST("/apk/upload/assemble", authMiddleware(), handleAssembleChunks)
	// Admin: list all APK versions (requires authentication)
	c.Group.GET("/apk/versions", authMiddleware(), handleListApkVersions)
	// Public: check whether a newer APK is available
	c.Group.GET("/apk/check", handleCheckApkUpdate)
	// Public: download a specific APK by UUID token (prevents sequential enumeration)
	c.Group.GET("/apk/download/:token", handleDownloadApk)
}

func apkStoragePath() string {
	if p := os.Getenv("APK_STORAGE_PATH"); p != "" {
		return p
	}
	return "./apk_storage"
}

// handleUploadApk accepts a multipart/form-data request with fields:
//   - file        (required) – the APK binary
//   - version     (optional) – override the version extracted from the APK, must be MAJOR.MINOR.PATCH
//   - description (optional)
//
// The handler automatically extracts package name, version code, min/target SDK
// from the APK's AndroidManifest.xml.
func handleUploadApk(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, "file is required")
		return
	}

	storagePath := apkStoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		respondError(c, http.StatusInternalServerError, "could not create storage directory")
		return
	}

	// Use a random temporary name to avoid path traversal via the user-supplied filename.
	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		respondError(c, http.StatusInternalServerError, "could not save file")
		return
	}

	// Parse metadata from the APK.
	apkInfo, parseErr := apkutil.ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		respondError(c, http.StatusBadRequest, "could not parse APK metadata: "+parseErr.Error())
		return
	}

	// Determine the version: explicit form field overrides the APK's versionName.
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
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		respondError(c, http.StatusInternalServerError, "could not finalize file")
		return
	}

	description := c.PostForm("description")
	apk, err := repository.CreateApkVersion(
		version, fileName, destPath, description, apkInfo.PackageName,
		fileHeader.Size, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		respondError(c, http.StatusConflict, "could not save APK version: "+err.Error())
		return
	}

	respondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
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
		DownloadUrl:      fmt.Sprintf("/api/v1/apk/download/%s", apk.DownloadToken),
		CreatedAt:        apk.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleUploadChunk receives a single file chunk and stores it in a temporary directory.
// This endpoint is part of the chunked upload flow: the frontend splits large files into
// 10 MB pieces so that each POST stays below Cloudflare's 50 MB body-size limit.
// Responses are plain JSON (not CBOR) for maximum throughput.
//
// Form fields:
//   - upload_id    (required) – UUID identifying the upload session
//   - chunk_index  (required) – 0-based index of this chunk
//   - total_chunks (required) – total number of chunks for this upload
//   - file         (required) – the chunk binary data
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

	// Build the chunk directory path and verify it stays within the storage root.
	chunkDir := filepath.Join(apkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(apkStoragePath())
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

// handleAssembleChunks joins all previously uploaded chunks into a single APK file,
// parses its metadata, and creates the APK version record – exactly as handleUploadApk does.
// Responses are plain JSON (not CBOR) to match the chunk upload endpoint.
//
// Form fields:
//   - upload_id    (required) – UUID identifying the upload session
//   - total_chunks (required) – total number of chunks expected
//   - version      (optional) – override version extracted from APK (MAJOR.MINOR.PATCH)
//   - description  (optional)
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

	// Validate the chunk directory stays within the storage root.
	chunkDir := filepath.Join(apkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(apkStoragePath())
	absChunkDir, err := filepath.Abs(chunkDir)
	if err != nil || !strings.HasPrefix(absChunkDir, absStoragePath+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	// Verify that every expected chunk is present before starting assembly.
	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", i))
		if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
			c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("chunk %d is missing", i)})
			return
		}
	}

	storagePath := apkStoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create storage directory"})
		return
	}

	// Create a temporary file to stream the assembled content into.
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

	// Capture total file size before closing.
	totalSize := int64(0)
	if info, err := outFile.Stat(); err == nil {
		totalSize = info.Size()
	}
	outFile.Close()

	// Clean up chunk directory now that assembly is complete.
	_ = os.RemoveAll(chunkDir)

	// Parse APK metadata from the assembled file.
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

	respondProto(c, http.StatusCreated, &pb.ApkUploadResponse{
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
		DownloadUrl:      fmt.Sprintf("/api/v1/apk/download/%s", apk.DownloadToken),
		CreatedAt:        apk.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleCheckApkUpdate compares the client's current version against the latest APK on the server.
// Query param: version (e.g. "1.0.0")
// Returns update_available=true with metadata when the server has a newer version.
func handleCheckApkUpdate(c *gin.Context) {
	clientVersion := c.Query("version")
	clientPackage := c.Query("package")
	if clientVersion == "" {
		respondError(c, http.StatusBadRequest, "version query parameter is required")
		return
	}

	if clientPackage == "" {
		respondError(c, http.StatusBadRequest, "package query parameter is required")
		return
	}

	latest, err := repository.GetLatestApkVersion(clientPackage)
	if err != nil {
		respondError(c, http.StatusNotFound, "no APK version available")
		return
	}

	newer, err := repository.IsNewerVersion(clientVersion, latest.Version)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp := &pb.ApkUpdateCheckResponse{
		UpdateAvailable: newer,
		LatestVersion:   latest.Version,
		PackageName:     latest.PackageName,
		VersionCode:     latest.VersionCode,
	}
	if newer {
		resp.DownloadUrl = fmt.Sprintf("/api/v1/apk/download/%s", latest.DownloadToken)
		resp.Description = latest.Description
		resp.FileSize = latest.FileSize
		resp.MinSdkVersion = latest.MinSDKVersion
		resp.TargetSdkVersion = latest.TargetSDKVersion
	}

	respondProto(c, http.StatusOK, resp)
}

// handleDownloadApk streams the APK file identified by its UUID download token.
func handleDownloadApk(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		respondError(c, http.StatusBadRequest, "invalid download token")
		return
	}

	apk, err := repository.GetApkVersionByToken(token)
	if err != nil {
		respondError(c, http.StatusNotFound, "APK version not found")
		return
	}

	// Ensure the stored path is still within the designated storage directory.
	storagePath, _ := filepath.Abs(apkStoragePath())
	absPath, err := filepath.Abs(apk.FilePath)
	if err != nil || !strings.HasPrefix(absPath, storagePath+string(filepath.Separator)) {
		respondError(c, http.StatusForbidden, "file path is invalid")
		return
	}

	c.FileAttachment(absPath, apk.FileName)
}

// handleListApkVersions returns all uploaded APK versions.
func handleListApkVersions(c *gin.Context) {
	versions, err := repository.ListApkVersions()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondProto(c, http.StatusOK, &pb.ApkList{Versions: apksToProto(versions)})
}
