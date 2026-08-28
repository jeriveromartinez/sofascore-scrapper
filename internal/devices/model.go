package devices

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

type Device struct {
	gorm.Model
	UserID   *uint
	Token    string `gorm:"uniqueIndex;not null"`
	Platform string
	Name     string
	LastSeen int64
	Version  string
	Manager  *users.User `gorm:"foreignKey:UserID"`

	// DomainID links the device to a user-owned domain. Push audiences
	// are matched on this column. Nullable: a device registered before
	// the push feature shipped (or that never picked a domain) stays
	// NULL and is excluded from push delivery.
	DomainID *uint           `gorm:"index;null"`
	Domain   *domains.Domain `gorm:"foreignKey:DomainID"`
}
