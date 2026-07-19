package apk

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(version, fileName, filePath, description, packageName string, fileSize int64, versionCode, minSDK, targetSDK int32) (*ApkVersion, error) {
	apk := &ApkVersion{
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

	lastVersion, err := r.GetLatest(apk.PackageName)
	if strings.TrimSpace(apk.IPTVUrl) == "" && err == nil && lastVersion != nil {
		apk.IPTVUrl = lastVersion.IPTVUrl
	}

	if err := r.db.Create(apk).Error; err != nil {
		return nil, err
	}

	return apk, nil
}

func (r *Repository) GetLatest(packageName string) (*ApkVersion, error) {
	var versions []ApkVersion
	if err := r.db.Where("is_active = ? AND package_name = ?", true, packageName).Find(&versions).Error; err != nil {
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

func (r *Repository) GetByID(id uint) (*ApkVersion, error) {
	var apk ApkVersion
	if err := r.db.First(&apk, id).Error; err != nil {
		return nil, err
	}
	return &apk, nil
}

func (r *Repository) GetByToken(token string) (*ApkVersion, error) {
	var apk ApkVersion
	if err := r.db.Where("download_token = ?", token).First(&apk).Error; err != nil {
		return nil, err
	}
	return &apk, nil
}

func (r *Repository) ListAll() ([]ApkVersion, error) {
	var versions []ApkVersion
	if err := r.db.Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *Repository) ListPage(ctx context.Context, createdAtStr string, id uint, limit int) ([]ApkVersion, bool, error) {
	query := r.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if createdAtStr != "" {
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, false, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", createdAt, createdAt, id)
	}
	var rows []ApkVersion
	err := query.Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

func (r *Repository) UpdateURL(id uint, url string) error {
	result := r.db.Model(&ApkVersion{}).Where("id = ?", id).Update("iptv_url", url)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) IncrementDownloadCount(id uint) error {
	return r.db.Model(&ApkVersion{}).Where("id = ?", id).UpdateColumn("total_downloads", gorm.Expr("total_downloads + ?", 1)).Error
}
