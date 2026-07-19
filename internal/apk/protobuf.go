package apk

import (
	"fmt"
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

func ApkToProto(v ApkVersion, downloadURL string) *pb.ApkInfo {
	return &pb.ApkInfo{
		Id:               uint32(v.ID),
		Version:          v.Version,
		FileName:         v.FileName,
		FileSize:         v.FileSize,
		Description:      v.Description,
		IsActive:         v.IsActive,
		PackageName:      v.PackageName,
		VersionCode:      v.VersionCode,
		MinSdkVersion:    v.MinSDKVersion,
		TargetSdkVersion: v.TargetSDKVersion,
		DownloadToken:    v.DownloadToken,
		DownloadUrl:      downloadURL,
		CreatedAt:        v.CreatedAt.Format(time.RFC3339),
		Downloads:        int32(v.TotalDownloads),
		PanelUrl:         v.IPTVUrl,
	}
}

func ApksToProto(versions []ApkVersion) []*pb.ApkInfo {
	result := make([]*pb.ApkInfo, 0, len(versions))
	for _, v := range versions {
		result = append(result, ApkToProto(v, fmt.Sprintf("/api/app/v1/apk/download/%s", v.DownloadToken)))
	}
	return result
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
