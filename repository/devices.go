package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func RegisterDevice(userID uint, token, platform, name string) (*models.Device, error) {
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
