package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func apkRepo() (*apk.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return apk.NewRepository(db), nil
}

func CreateApkVersion(version, fileName, filePath, description, packageName string, fileSize int64, versionCode, minSDK, targetSDK int32) (*apk.ApkVersion, error) {
	repo, err := apkRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(version, fileName, filePath, description, packageName, fileSize, versionCode, minSDK, targetSDK)
}

func GetLatestApkVersion(packageName string) (*apk.ApkVersion, error) {
	repo, err := apkRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetLatest(packageName)
}

func GetApkVersionByID(id uint) (*apk.ApkVersion, error) {
	repo, err := apkRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByID(id)
}

func GetApkVersionByToken(token string) (*apk.ApkVersion, error) {
	repo, err := apkRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByToken(token)
}

func ListApkVersions() ([]apk.ApkVersion, error) {
	repo, err := apkRepo()
	if err != nil {
		return nil, err
	}
	return repo.ListAll()
}

func UpdateApkUrl(id uint, url string) error {
	repo, err := apkRepo()
	if err != nil {
		return err
	}
	return repo.UpdateURL(id, url)
}

func UpdateDownloadCount(id uint) error {
	repo, err := apkRepo()
	if err != nil {
		return err
	}
	return repo.IncrementDownloadCount(id)
}

func IsNewerVersion(current, candidate string) (bool, error) {
	return apk.IsNewerVersion(current, candidate)
}
