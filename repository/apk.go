package repository

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

// CreateApkVersion persists a new APK version record.
func CreateApkVersion(version, fileName, filePath, description, packageName string, fileSize int64, versionCode, minSDK, targetSDK int32) (*models.ApkVersion, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	apk := &models.ApkVersion{
		Version:          version,
		FileName:         fileName,
		FilePath:         filePath,
		FileSize:         fileSize,
		Description:      description,
		IsActive:         true,
		PackageName:      packageName,
		VersionCode:      versionCode,
		MinSDKVersion:    minSDK,
		TargetSDKVersion: targetSDK,
		DownloadToken:    uuid.New().String(),
	}
	if err := db.Create(apk).Error; err != nil {
		return nil, err
	}
	return apk, nil
}

// GetLatestApkVersion returns the active APK with the highest semantic version.
func GetLatestApkVersion(packageName string) (*models.ApkVersion, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var versions []models.ApkVersion
	if err := db.Where("is_active = ? AND package_name = ?", true, packageName).Find(&versions).Error; err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, errors.New("no active APK version found")
	}

	latest := &versions[0]
	for i := 1; i < len(versions); i++ {
		newer, err := IsNewerVersion(latest.Version, versions[i].Version)
		if err != nil {
			continue
		}
		if newer {
			latest = &versions[i]
		}
	}
	return latest, nil
}

// GetApkVersionByID returns a specific APK version by its primary key.
func GetApkVersionByID(id uint) (*models.ApkVersion, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var apk models.ApkVersion
	if err := db.First(&apk, id).Error; err != nil {
		return nil, err
	}
	return &apk, nil
}

// GetApkVersionByToken returns the APK version associated with the given UUID
// download token. Returns an error if no matching record is found.
func GetApkVersionByToken(token string) (*models.ApkVersion, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var apk models.ApkVersion
	if err := db.Where("download_token = ?", token).First(&apk).Error; err != nil {
		return nil, err
	}
	return &apk, nil
}

// ListApkVersions returns all APK versions ordered by creation date descending.
func ListApkVersions() ([]models.ApkVersion, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var versions []models.ApkVersion
	if err := db.Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

// IsNewerVersion returns true when candidate is strictly newer than current.
// Both strings must be in "MAJOR.MINOR.PATCH" format.
func IsNewerVersion(current, candidate string) (bool, error) {
	cur, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	can, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("invalid candidate version %q: %w", candidate, err)
	}
	for i := range cur {
		if can[i] > cur[i] {
			return true, nil
		}
		if can[i] < cur[i] {
			return false, nil
		}
	}
	return false, nil
}

func parseVersion(v string) ([3]int, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, errors.New("version must have exactly 3 components (MAJOR.MINOR.PATCH)")
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("component %d is not a non-negative integer", i)
		}
		result[i] = n
	}
	return result, nil
}
