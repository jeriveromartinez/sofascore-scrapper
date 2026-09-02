package devices

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

type Device struct {
	gorm.Model
	UserID    *uint           `gorm:"column:user_id;index:idx_devices_user_id"`
	Token     string          `gorm:"column:token;size:191;uniqueIndex:idx_devices_token;not null"`
	Platform  string          `gorm:"column:platform;type:longtext"`
	Name      string          `gorm:"column:name;type:longtext"`
	PackageId string          `gorm:"column:package_id;type:longtext"`
	LastSeen  int64           `gorm:"column:last_seen"`
	Version   string          `gorm:"column:version;type:longtext"`
	Manager   *users.User     `gorm:"foreignKey:UserID;references:ID"`
	DomainID  *uint           `gorm:"column:domain_id;index:idx_devices_domain_id;null;foreignKey:DomainID;references:ID;constraint:OnDelete:SET_NULL,OnUpdate:CASCADE"`
	Domain    *domains.Domain `gorm:"foreignKey:DomainID;references:ID"`
}
