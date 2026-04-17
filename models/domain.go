package models

import "gorm.io/gorm"

type Domain struct {
	gorm.Model
	Domain string `gorm:"uniqueIndex;not null"`
	UserID uint   `gorm:"index;not null"`
	User   *User  `gorm:"foreignKey:UserID"`
}
