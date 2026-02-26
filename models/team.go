package models

import "gorm.io/gorm"

type Team struct {
	gorm.Model
	TeamId  int64 `gorm:"uniqueIndex"`
	LogoUrl string
}
