package models

import "time"

import "gorm.io/gorm"

type RefreshToken struct {
	gorm.Model
	UserID    uint       `gorm:"index;not null"`
	TokenID   string     `gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time  `gorm:"index;not null"`
	RevokedAt *time.Time `gorm:"index"`
}
