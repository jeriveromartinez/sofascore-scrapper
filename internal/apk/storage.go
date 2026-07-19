package apk

import "os"

func StoragePath() string {
	if p := os.Getenv("APK_STORAGE_PATH"); p != "" {
		return p
	}
	return "./apk_storage"
}
