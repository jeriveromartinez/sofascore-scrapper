package devices

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

const maxAllDevices = 1000

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Register upserts a device by token. If the row already exists, only
// the explicitly provided fields are refreshed; a nil domainID does
// NOT clear an existing domain_id, so a re-register from the Flutter
// app does not wipe the operator's domain assignment.
func (r *Repository) Register(userID, domainID *uint, token, platform, name, version string) (*Device, error) {
	device := &Device{
		UserID:   userID,
		Token:    token,
		Platform: platform,
		Name:     name,
		Version:  version,
		LastSeen: time.Now().Unix(),
	}
	assign := Device{UserID: userID, Platform: platform, Name: name, LastSeen: device.LastSeen, Version: version}
	if domainID != nil {
		assign.DomainID = domainID
	}
	result := r.db.Where(Device{Token: token}).Assign(assign).FirstOrCreate(device)
	return device, result.Error
}

func (r *Repository) UpdateLastSeen(token string) error {
	return r.db.Model(&Device{}).Where("token = ?", token).Update("last_seen", time.Now().Unix()).Error
}

func (r *Repository) UpdateLastSeenByID(deviceID uint, ts int64) error {
	return r.db.Model(&Device{}).Where("id = ?", deviceID).Update("last_seen", ts).Error
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
	result := r.db.Order("id DESC").Limit(maxAllDevices).Preload("Manager").Find(&devices)
	return devices, result.Error
}

func (r *Repository) ListPage(ctx context.Context, id uint, limit int) ([]Device, bool, error) {
	query := r.db.WithContext(ctx).Preload("Manager").Order("id DESC")
	if id > 0 {
		query = query.Where("id < ?", id)
	}
	var rows []Device
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
