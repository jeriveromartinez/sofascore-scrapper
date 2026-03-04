package models

import "gorm.io/gorm"

// ApkVersion stores metadata for each uploaded APK release.
type ApkVersion struct {
	gorm.Model
	Version     string `gorm:"uniqueIndex;not null"`
	FileName    string `gorm:"not null"`
	FilePath    string `gorm:"not null"`
	FileSize    int64
	Description string
	IsActive    bool `gorm:"default:true"`
}
