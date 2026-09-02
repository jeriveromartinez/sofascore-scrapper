package apk

import "gorm.io/gorm"

type ApkVersion struct {
	gorm.Model
	Version          string `gorm:"column:version;size:191;uniqueIndex:idx_apk_version_package,priority:1;not null"`
	FileName         string `gorm:"column:file_name;type:longtext;not null"`
	FilePath         string `gorm:"column:file_path;type:longtext;not null"`
	FileSize         int64  `gorm:"column:file_size"`
	Description      string `gorm:"column:description;type:longtext"`
	IsActive         bool   `gorm:"column:is_active;not null;default:true;index:idx_apk_latest,priority:2"`
	PackageName      string `gorm:"column:package_name;size:191;uniqueIndex:idx_apk_version_package,priority:2;index:idx_apk_latest,priority:1,length:191;not null"`
	VersionCode      int32  `gorm:"column:version_code"`
	MinSDKVersion    int32  `gorm:"column:min_sdk_version"`
	TargetSDKVersion int32  `gorm:"column:target_sdk_version"`
	DownloadToken    string `gorm:"column:download_token;size:191;uniqueIndex:idx_apk_versions_download_token;not null"`
	TotalDownloads   int64  `gorm:"column:total_downloads;default:0"`
	IPTVUrl          string `gorm:"column:iptv_url;type:longtext;default:'http://5.mdtv.me'"`
	VersionMajor     uint64 `gorm:"column:version_major;not null;default:0;index:idx_apk_latest,priority:3"`
	VersionMinor     uint64 `gorm:"column:version_minor;not null;default:0;index:idx_apk_latest,priority:4"`
	VersionPatch     uint64 `gorm:"column:version_patch;not null;default:0;index:idx_apk_latest,priority:5"`
}
