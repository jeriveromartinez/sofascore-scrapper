package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func RegisterDevice(userID *uint, token, platform, name string) (*models.Device, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	device := &models.Device{
		UserID:   userID,
		Token:    token,
		Platform: platform,
		Name:     name,
		LastSeen: time.Now().Unix(),
	}
	result := db.Where(models.Device{Token: token}).Assign(models.Device{UserID: userID, Platform: platform, Name: name, LastSeen: device.LastSeen}).FirstOrCreate(device)
	return device, result.Error
}

func UpdateDeviceLastSeen(token string) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Model(&models.Device{}).Where("token = ?", token).Update("last_seen", time.Now().Unix()).Error
}

func GetDevices(page, limit uint) ([]models.Device, int64, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, 0, err
	}
	var devices []models.Device
	var total int64
	if err := db.Model(&models.Device{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	result := db.Offset(int(offset)).Limit(int(limit)).Preload("Manager").Find(&devices)
	return devices, total, result.Error
}

func GetAllDevices() ([]models.Device, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var devices []models.Device
	result := db.Preload("Manager").Find(&devices)
	return devices, result.Error
}
