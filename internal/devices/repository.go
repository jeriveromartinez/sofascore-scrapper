package devices

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/apk"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

const (
	maxAllDevices = 1000
	// maxTokenLength matches devices.token varchar(191). A longer token
	// would fail the INSERT with a driver error (MySQL 1406) instead of a
	// client error.
	maxTokenLength = 191
)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// escapeLike escapes the LIKE special characters (% multi-char wildcard,
// _ single-char wildcard) and the escape character itself (!) using ! as
// the LIKE escape character, so a host name taken from an APK row cannot
// widen the pattern it is embedded in.
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '!' || r == '%' || r == '_' {
			b.WriteByte('!')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (r *Repository) Register(ctx context.Context, userID *uint, domainID uint, token, platform, name, version, packageId, timezone string) (*Device, error) {
	if packageId == "" || token == "" || len(token) > maxTokenLength {
		return nil, gorm.ErrInvalidData
	}
	db := r.db.WithContext(ctx)

	assignedDomainID, err := r.resolveDomainID(db, packageId, domainID)
	if err != nil {
		return nil, err
	}

	lastSeen := time.Now().Unix()
	device := &Device{UserID: userID, Token: token, Platform: platform, Name: name, Version: version, LastSeen: lastSeen, PackageId: packageId, DomainID: &assignedDomainID, Timezone: timezone}
	assign := Device{UserID: userID, Platform: platform, Name: name, LastSeen: lastSeen, Version: version, PackageId: packageId, DomainID: &assignedDomainID, Timezone: timezone}

	if err := upsertDevice(db, device, assign); err != nil {
		return nil, err
	}
	return device, nil
}

// resolveDomainID returns the domain the device will be pushed to: the
// explicit domain_id sent by the app when it is non-zero, otherwise the
// domain derived from the IPTV URL of the APK version currently served.
func (r *Repository) resolveDomainID(db *gorm.DB, packageID string, explicitID uint) (uint, error) {
	if explicitID != 0 {
		var domain domains.Domain
		if err := db.First(&domain, explicitID).Error; err != nil {
			return 0, invalidDataUnlessDBError(err)
		}
		return domain.ID, nil
	}

	var apkDev apk.ApkVersion
	// Match APK by exact package_name (not LIKE). packageId comes from
	// the public device-registration endpoint, so an attacker could
	// otherwise send "%" and bind the device to whichever APK row the
	// planner picks first — leaking the chosen domain as the device's
	// future push audience. Only active rows are candidates, ordered
	// like apk.Repository.GetLatest so the derived domain is the one the
	// device is actually served.
	if err := db.Where("package_name = ? AND is_active = ?", packageID, true).
		Order("version_major DESC, version_minor DESC, version_patch DESC, id DESC").
		Limit(1).
		First(&apkDev).Error; err != nil {
		return 0, invalidDataUnlessDBError(err)
	}
	host := hostFromIPTVURL(apkDev.IPTVUrl)
	if host == "" {
		return 0, gorm.ErrInvalidData
	}

	var domain domains.Domain
	err := db.Where("domain = ?", host).First(&domain).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Fall back to domains registered below the APK host
		// (panel.5.mdtv.me for host 5.mdtv.me). The leading dot keeps
		// panel5.mdtv.me from matching.
		err = db.Where("domain LIKE ? ESCAPE '!'", "%."+escapeLike(host)).First(&domain).Error
	}
	if err != nil {
		return 0, invalidDataUnlessDBError(err)
	}
	return domain.ID, nil
}

// hostFromIPTVURL extracts the bare host (no port, no path) from an IPTV
// URL. Older rows store the host without a scheme, so the raw string is
// kept when parsing cannot recover a host.
func hostFromIPTVURL(raw string) string {
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if _, after, found := strings.Cut(raw, "://"); found {
		return after
	}
	return raw
}

// upsertDevice keeps registration idempotent: an existing token updates its
// row, a soft-deleted row with the same token is restored (the unique index
// on token covers deleted rows too), and losing the insert race to a
// concurrent request falls back to the update path.
func upsertDevice(db *gorm.DB, device *Device, assign Device) error {
	var existing Device
	err := db.Unscoped().Where("token = ?", device.Token).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.Create(device).Error
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
		if err := db.Unscoped().Where("token = ?", device.Token).First(&existing).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if existing.DeletedAt.Valid {
		if err := db.Unscoped().Model(&existing).Update("deleted_at", nil).Error; err != nil {
			return err
		}
		existing.DeletedAt = gorm.DeletedAt{}
	}
	if err := db.Unscoped().Model(&existing).Updates(assign).Error; err != nil {
		return err
	}
	*device = existing
	return nil
}

// isUniqueViolation reports a duplicate-key failure from MySQL/MariaDB
// (1062) or SQLite, the only errors Register treats as a lost insert race.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1062") || strings.Contains(msg, "UNIQUE constraint failed")
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

func (r *Repository) GetDevices(ctx context.Context, page, limit uint) ([]Device, int64, error) {
	db := r.db.WithContext(ctx)
	var devices []Device
	var total int64
	if err := db.Model(&Device{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	// The pb.DeviceList proto never exposes the joined users.User
	// fields, so the device → user relation is never loaded here.
	// If a future consumer needs the manager email, use a LEFT JOIN
	// users that selects only that column rather than loading the
	// whole row.
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
