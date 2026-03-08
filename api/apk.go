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
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}

	storagePath := apkStoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not create storage directory"})
		return
	}

	// Use a random temporary name to avoid path traversal via the user-supplied filename.
	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not save file"})
		return
	}

	// Parse metadata from the APK.
	apkInfo, parseErr := apkutil.ParseAPKInfo(tmpPath)
	if parseErr != nil {
		_ = os.Remove(tmpPath)
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "could not parse APK metadata: " + parseErr.Error()})
		return
	}

	// Determine the version: explicit form field overrides the APK's versionName.
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
		respondCBOR(c, http.StatusBadRequest, errResp)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not finalize file"})
		return
	}

	description := c.PostForm("description")
	apk, err := repository.CreateApkVersion(
		version, fileName, destPath, description, apkInfo.PackageName,
		fileHeader.Size, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		respondCBOR(c, http.StatusConflict, map[string]string{"error": "could not save APK version: " + err.Error()})
		return
	}

	respondCBOR(c, http.StatusCreated, map[string]any{
		"id":                 apk.ID,
		"version":            apk.Version,
		"file_name":          apk.FileName,
		"file_size":          apk.FileSize,
		"description":        apk.Description,
		"package_name":       apk.PackageName,
		"version_code":       apk.VersionCode,
		"min_sdk_version":    apk.MinSDKVersion,
		"target_sdk_version": apk.TargetSDKVersion,
		"download_token":     apk.DownloadToken,
		"download_url":       fmt.Sprintf("/api/v1/apk/download/%s", apk.DownloadToken),
		"created_at":         apk.CreatedAt,
	})
}

// handleUploadChunk receives a single file chunk and stores it in a temporary directory.
// This endpoint is part of the chunked upload flow: the frontend splits large files into
// 10 MB pieces so that each POST stays below Cloudflare's 50 MB body-size limit.
//
// Form fields:
//   - upload_id    (required) – UUID identifying the upload session
//   - chunk_index  (required) – 0-based index of this chunk
//   - total_chunks (required) – total number of chunks for this upload
//   - file         (required) – the chunk binary data
func handleUploadChunk(c *gin.Context) {
	uploadID := c.PostForm("upload_id")
	if _, err := uuid.Parse(uploadID); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	chunkIndex, err := strconv.Atoi(c.PostForm("chunk_index"))
	if err != nil || chunkIndex < 0 {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid chunk_index"})
		return
	}

	totalChunks, err := strconv.Atoi(c.PostForm("total_chunks"))
	if err != nil || totalChunks <= 0 || totalChunks > maxTotalChunks {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid total_chunks"})
		return
	}

	if chunkIndex >= totalChunks {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "chunk_index must be less than total_chunks"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "chunk data is required"})
		return
	}

	if fileHeader.Size > maxChunkSize {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "chunk size exceeds maximum allowed size"})
		return
	}

	// Build the chunk directory path and verify it stays within the storage root.
	chunkDir := filepath.Join(apkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(apkStoragePath())
	absChunkDir, err := filepath.Abs(chunkDir)
	if err != nil || !strings.HasPrefix(absChunkDir, absStoragePath+string(filepath.Separator)) {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not create chunk directory"})
		return
	}

	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", chunkIndex))
	if err := c.SaveUploadedFile(fileHeader, chunkPath); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not save chunk"})
		return
	}

	respondCBOR(c, http.StatusOK, map[string]any{
		"upload_id":    uploadID,
		"chunk_index":  chunkIndex,
		"total_chunks": totalChunks,
	})
}

// handleAssembleChunks joins all previously uploaded chunks into a single APK file,
// parses its metadata, and creates the APK version record – exactly as handleUploadApk does.
//
// Form fields:
//   - upload_id    (required) – UUID identifying the upload session
//   - total_chunks (required) – total number of chunks expected
//   - version      (optional) – override version extracted from APK (MAJOR.MINOR.PATCH)
//   - description  (optional)
func handleAssembleChunks(c *gin.Context) {
	uploadID := c.PostForm("upload_id")
	if _, err := uuid.Parse(uploadID); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	totalChunks, err := strconv.Atoi(c.PostForm("total_chunks"))
	if err != nil || totalChunks <= 0 || totalChunks > maxTotalChunks {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid total_chunks"})
		return
	}

	// Validate the chunk directory stays within the storage root.
	chunkDir := filepath.Join(apkStoragePath(), "chunks", uploadID)
	absStoragePath, _ := filepath.Abs(apkStoragePath())
	absChunkDir, err := filepath.Abs(chunkDir)
	if err != nil || !strings.HasPrefix(absChunkDir, absStoragePath+string(filepath.Separator)) {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid upload_id"})
		return
	}

	// Verify that every expected chunk is present before starting assembly.
	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", i))
		if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
			respondCBOR(c, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("chunk %d is missing", i)})
			return
		}
	}

	storagePath := apkStoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not create storage directory"})
		return
	}

	// Create a temporary file to stream the assembled content into.
	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-tmp-%d.apk", randomSuffix()))
	outFile, err := os.Create(tmpPath)
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not create temporary file"})
		return
	}

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d", i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			outFile.Close()
			_ = os.Remove(tmpPath)
			respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("could not read chunk %d", i)})
			return
		}
		_, copyErr := io.Copy(outFile, chunkFile)
		chunkFile.Close()
		if copyErr != nil {
			outFile.Close()
			_ = os.Remove(tmpPath)
			respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not write assembled file"})
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
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "could not parse APK metadata: " + parseErr.Error()})
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
		respondCBOR(c, http.StatusBadRequest, errResp)
		return
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath := filepath.Join(storagePath, fileName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": "could not finalize file"})
		return
	}

	description := c.PostForm("description")
	apk, err := repository.CreateApkVersion(
		version, fileName, destPath, description, apkInfo.PackageName,
		totalSize, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		_ = os.Remove(destPath)
		respondCBOR(c, http.StatusConflict, map[string]string{"error": "could not save APK version: " + err.Error()})
		return
	}

	respondCBOR(c, http.StatusCreated, map[string]any{
		"id":                 apk.ID,
		"version":            apk.Version,
		"file_name":          apk.FileName,
		"file_size":          apk.FileSize,
		"description":        apk.Description,
		"package_name":       apk.PackageName,
		"version_code":       apk.VersionCode,
		"min_sdk_version":    apk.MinSDKVersion,
		"target_sdk_version": apk.TargetSDKVersion,
		"download_token":     apk.DownloadToken,
		"download_url":       fmt.Sprintf("/api/v1/apk/download/%s", apk.DownloadToken),
		"created_at":         apk.CreatedAt,
	})
}

// handleCheckApkUpdate compares the client's current version against the latest APK on the server.
// Query param: version (e.g. "1.0.0")
// Returns update_available=true with metadata when the server has a newer version.
func handleCheckApkUpdate(c *gin.Context) {
	clientVersion := c.Query("version")
	if clientVersion == "" {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "version query parameter is required"})
		return
	}

	latest, err := repository.GetLatestApkVersion()
	if err != nil {
		respondCBOR(c, http.StatusNotFound, map[string]string{"error": "no APK version available"})
		return
	}

	newer, err := repository.IsNewerVersion(clientVersion, latest.Version)
	if err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]any{
		"update_available": newer,
		"latest_version":   latest.Version,
		"package_name":     latest.PackageName,
		"version_code":     latest.VersionCode,
	}
	if newer {
		resp["download_url"] = fmt.Sprintf("/api/v1/apk/download/%s", latest.DownloadToken)
		resp["description"] = latest.Description
		resp["file_size"] = latest.FileSize
		resp["min_sdk_version"] = latest.MinSDKVersion
		resp["target_sdk_version"] = latest.TargetSDKVersion
	}

	respondCBOR(c, http.StatusOK, resp)
}

// handleDownloadApk streams the APK file identified by its UUID download token.
func handleDownloadApk(c *gin.Context) {
	token := c.Param("token")
	if _, err := uuid.Parse(token); err != nil {
		respondCBOR(c, http.StatusBadRequest, map[string]string{"error": "invalid download token"})
		return
	}

	apk, err := repository.GetApkVersionByToken(token)
	if err != nil {
		respondCBOR(c, http.StatusNotFound, map[string]string{"error": "APK version not found"})
		return
	}

	// Ensure the stored path is still within the designated storage directory.
	storagePath, _ := filepath.Abs(apkStoragePath())
	absPath, err := filepath.Abs(apk.FilePath)
	if err != nil || !strings.HasPrefix(absPath, storagePath+string(filepath.Separator)) {
		respondCBOR(c, http.StatusForbidden, map[string]string{"error": "file path is invalid"})
		return
	}

	c.FileAttachment(absPath, apk.FileName)
}

// handleListApkVersions returns all uploaded APK versions.
func handleListApkVersions(c *gin.Context) {
	versions, err := repository.ListApkVersions()
	if err != nil {
		respondCBOR(c, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type apkInfo struct {
		ID               uint   `json:"id" cbor:"id"`
		Version          string `json:"version" cbor:"version"`
		FileName         string `json:"file_name" cbor:"file_name"`
		FileSize         int64  `json:"file_size" cbor:"file_size"`
		Description      string `json:"description" cbor:"description"`
		IsActive         bool   `json:"is_active" cbor:"is_active"`
		PackageName      string `json:"package_name" cbor:"package_name"`
		VersionCode      int32  `json:"version_code" cbor:"version_code"`
		MinSDKVersion    int32  `json:"min_sdk_version" cbor:"min_sdk_version"`
		TargetSDKVersion int32  `json:"target_sdk_version" cbor:"target_sdk_version"`
		DownloadToken    string `json:"download_token" cbor:"download_token"`
		DownloadURL      string `json:"download_url" cbor:"download_url"`
		CreatedAt        any    `json:"created_at" cbor:"created_at"`
	}

	result := make([]apkInfo, 0, len(versions))
	for _, v := range versions {
		result = append(result, apkInfo{
			ID:               v.ID,
			Version:          v.Version,
			FileName:         v.FileName,
			FileSize:         v.FileSize,
			Description:      v.Description,
			IsActive:         v.IsActive,
			PackageName:      v.PackageName,
			VersionCode:      v.VersionCode,
			MinSDKVersion:    v.MinSDKVersion,
			TargetSDKVersion: v.TargetSDKVersion,
			DownloadToken:    v.DownloadToken,
			DownloadURL:      fmt.Sprintf("/api/v1/apk/download/%s", v.DownloadToken),
			CreatedAt:        v.CreatedAt,
		})
	}

	respondCBOR(c, http.StatusOK, result)
}

