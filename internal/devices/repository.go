package devices

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

const maxAllDevices = 1000

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// escapeLike escapes the three special characters recognised by SQL LIKE
// (% multi-char wildcard, _ single-char wildcard, \ literal escape char) by
// prefixing each with the LIKE escape character (backslash). This prevents a
// caller from constructing a wildcard query from arbitrary user input, per
// the events package's LIKE wildcard-injection hardening (see internal/events
// /repository.go).
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (r *Repository) Register(ctx context.Context, userID *uint, token, platform, name, version, packageId, timezone string) (*Device, error) {
	if packageId == "" || token == "" || name == "" {
		return nil, gorm.ErrInvalidData
	}
	db := r.db.WithContext(ctx)

	var apkDev apk.ApkVersion
	// Match APK by exact package_name (not LIKE). packageId comes from
	// the public device-registration endpoint, so an attacker could
	// otherwise send "%" and bind the device to whichever APK row the
	// planner picks first — leaking the chosen domain as the device's
	// future push audience.
	if err := db.Model(&apk.ApkVersion{}).Where("package_name = ?", packageId).First(&apkDev).Error; err != nil {
		return nil, invalidDataUnlessDBError(err)
	}
	if apkDev.IPTVUrl == "" {
		return nil, gorm.ErrInvalidData
	}

	var domainStr string
	parts := strings.Split(apkDev.IPTVUrl, "://")
	if len(parts) > 1 {
		domainStr = parts[1]
	} else {
		domainStr = apkDev.IPTVUrl
	}

	var domain domains.Domain
	// Like-escape the domain suffix before the LIKE query. A device-
	// controlled packageId flows into apkDev.IPTVUrl only when an
	// attacker has already poisoned the package_name -> IPTVUrl row,
	// but the trailing % here is a fixed literal, so the user input
	// the escape guards against is apkDev.IPTVUrl itself: an operator
	// that registers a domain containing "_" or "%" would otherwise
	// silently widen the match.
	if err := db.Model(&domains.Domain{}).Where("domain LIKE ? ESCAPE '\\'", "%"+escapeLike(domainStr)).First(&domain).Error; err != nil {
		return nil, invalidDataUnlessDBError(err)
	}

	device := &Device{UserID: userID, Token: token, Platform: platform, Name: name, Version: version, LastSeen: time.Now().Unix(), PackageId: packageId, DomainID: &domain.ID, Timezone: timezone}
	assign := Device{UserID: userID, Platform: platform, Name: name, LastSeen: device.LastSeen, Version: version, PackageId: packageId, DomainID: &domain.ID, Timezone: timezone}

	result := db.Where(Device{Token: token}).Assign(assign).FirstOrCreate(device)
	return device, result.Error
}

// invalidDataUnlessDBError keeps "no matching row" reported as invalid
// registration data while letting real failures (a cancelled request,
// a lost connection) surface as themselves.
func invalidDataUnlessDBError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return gorm.ErrInvalidData
	}
	return err
}

func (r *Repository) UpdateLastSeen(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Model(&Device{}).Where("token = ?", token).Update("last_seen", time.Now().Unix()).Error
}

func (r *Repository) UpdateLastSeenByID(ctx context.Context, deviceID uint, ts int64) error {
	return r.db.WithContext(ctx).Model(&Device{}).Where("id = ?", deviceID).Update("last_seen", ts).Error
}

func (r *Repository) GetDevices(ctx context.Context, page, limit uint) ([]Device, int64, error) {
	db := r.db.WithContext(ctx)
	var devices []Device
	var total int64
	if err := db.Model(&Device{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	// The pb.DeviceList proto never exposes the joined users.User
	// fields, so the Manager preload was a wasted N+1 roundtrip per
	// page. Dropped here and in ListPage/GetAll; if a future consumer
	// needs the manager email, replace with a LEFT JOIN users that
	// selects only that column rather than loading the whole row.
	result := db.Order("id DESC").Offset(int(offset)).Limit(int(limit)).Find(&devices)
	return devices, total, result.Error
}

func (r *Repository) GetAll(ctx context.Context) ([]Device, error) {
	var devices []Device
	result := r.db.WithContext(ctx).Order("id DESC").Limit(maxAllDevices).Find(&devices)
	return devices, result.Error
}

func (r *Repository) ListPage(ctx context.Context, id uint, limit int) ([]Device, bool, error) {
	query := r.db.WithContext(ctx).Order("id DESC")
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

func (r *Repository) FindByToken(ctx context.Context, token string) (*Device, error) {
	var device Device
	if err := r.db.WithContext(ctx).Where("token = ?", token).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *Repository) Update(ctx context.Context, token, platform, name, packageId, timezone string) (*Device, error) {
	var device Device
	if err := r.db.WithContext(ctx).Where("token = ?", token).First(&device).Error; err != nil {
		return nil, err
	}

	device.Platform = platform
	device.Name = name
	device.PackageId = packageId
	device.Timezone = timezone
	if err := r.db.WithContext(ctx).Save(&device).Error; err != nil {
		return nil, err
	}

	return &device, nil
}
