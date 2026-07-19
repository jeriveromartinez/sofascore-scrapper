package devices

import (
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
}
