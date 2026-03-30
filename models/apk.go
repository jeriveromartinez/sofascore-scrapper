package models

import "gorm.io/gorm"

type ApkVersion struct {
	gorm.Model
	Version          string `gorm:"uniqueIndex:idx_apk_version_package;not null"`
	FileName         string `gorm:"not null"`
	FilePath         string `gorm:"not null"`
	FileSize         int64
	Description      string
	IsActive         bool   `gorm:"default:true"`
	PackageName      string `gorm:"uniqueIndex:idx_apk_version_package;not null"`
	VersionCode      int32
	MinSDKVersion    int32
	TargetSDKVersion int32
	DownloadToken    string `gorm:"uniqueIndex;not null"`
	TotalDownloads   int64  `gorm:"default:0"`
}
