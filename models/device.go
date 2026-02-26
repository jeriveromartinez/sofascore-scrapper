package models

import "gorm.io/gorm"

type Device struct {
	gorm.Model
	UserID   uint   `gorm:"not null;index"`
	Token    string `gorm:"uniqueIndex;not null"`
	Platform string
	Name     string
	LastSeen int64
}
