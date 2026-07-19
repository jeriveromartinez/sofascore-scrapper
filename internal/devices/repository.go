package devices

import (
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Register(userID *uint, token, platform, name, version string) (*Device, error) {
	device := &Device{
		UserID:   userID,
		Token:    token,
		Platform: platform,
		Name:     name,
		Version:  version,
		LastSeen: time.Now().Unix(),
	}
	result := r.db.Where(Device{Token: token}).Assign(Device{UserID: userID, Platform: platform, Name: name, LastSeen: device.LastSeen, Version: version}).FirstOrCreate(device)
	return device, result.Error
}

func (r *Repository) UpdateLastSeen(token string) error {
	return r.db.Model(&Device{}).Where("token = ?", token).Update("last_seen", time.Now().Unix()).Error
}

func (r *Repository) GetDevices(page, limit uint) ([]Device, int64, error) {
	var devices []Device
	var total int64
	if err := r.db.Model(&Device{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	result := r.db.Offset(int(offset)).Limit(int(limit)).Preload("Manager").Find(&devices)
	return devices, total, result.Error
}

func (r *Repository) GetAll() ([]Device, error) {
	var devices []Device
	result := r.db.Preload("Manager").Find(&devices)
	return devices, result.Error
}

func (r *Repository) FindByToken(token string) (*Device, error) {
	var device Device
	if err := r.db.Where("token = ?", token).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *Repository) Update(token, platform, name string) (*Device, error) {
	var device Device
	if err := r.db.Where("token = ?", token).First(&device).Error; err != nil {
		return nil, err
	}

	device.Platform = platform
	device.Name = name
	if err := r.db.Save(&device).Error; err != nil {
		return nil, err
	}

	return &device, nil
}
