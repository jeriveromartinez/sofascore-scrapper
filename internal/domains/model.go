package domains

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

type Domain struct {
	gorm.Model
	Domain string      `gorm:"column:domain;size:191;uniqueIndex:idx_domains_domain;not null"`
	UserID uint        `gorm:"column:user_id;not null;index:idx_domains_user_id;foreignKey:UserID;references:ID"`
	User   *users.User `gorm:"foreignKey:UserID;references:ID"`
}
