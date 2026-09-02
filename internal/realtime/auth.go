package realtime

import (
	"context"
	"errors"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
)

// ErrInvalidToken is returned by AuthenticateToken when the supplied
// APP-XIPTV token does not match any device row. The upgrade handler
// translates this to a WS close with code 4401.
var ErrInvalidToken = errors.New("realtime: invalid device token")

// Authenticator validates the APP-XIPTV header (or ?token= query
// parameter for clients that cannot set headers) at WS upgrade time.
// The dependency is a *gorm.DB rather than a higher-level
// devices.Repository so the realtime package does not have to
// import the entire device domain — this keeps the package
// transport-focused and the test surface small.
type Authenticator struct {
	db *gorm.DB
}

// NewAuthenticator returns a new Authenticator. The db must be a
// fully wired GORM instance whose schema includes the devices
// table.
func NewAuthenticator(db *gorm.DB) *Authenticator { return &Authenticator{db: db} }

// AuthenticateToken looks up a device by token. It returns
// ErrInvalidToken when the row is missing, so the caller can
// distinguish "no such device" from a real database error (which
// is wrapped and returned as-is).
//
// A device with deleted_at set (GORM soft delete) is treated as
// invalid: GORM's default First() already filters out soft-deleted
// rows, so the missing-row branch covers both cases.
func (a *Authenticator) AuthenticateToken(ctx context.Context, token string) (*devices.Device, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	var dev devices.Device
	err := a.db.WithContext(ctx).Where("token = ?", token).First(&dev).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	return &dev, nil
}
