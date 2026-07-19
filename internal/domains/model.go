package domains

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

type Domain struct {
	gorm.Model
	Domain string     `gorm:"uniqueIndex;not null"`
	UserID uint       `gorm:"index;not null"`
	User   *users.User `gorm:"foreignKey:UserID"`
}
