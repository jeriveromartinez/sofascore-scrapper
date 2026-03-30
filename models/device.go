package models

import "gorm.io/gorm"

type Device struct {
	gorm.Model
	UserID   *uint
	Token    string `gorm:"uniqueIndex;not null"`
	Platform string
	Name     string
	LastSeen int64
	Version  string
	Manager  *User `gorm:"foreignKey:UserID"`
}
