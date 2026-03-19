package models

import "gorm.io/gorm"

type Team struct {
	gorm.Model
	TeamId         int64 `gorm:"uniqueIndex"`
	Name           string
	LogoUrl        string
	PrimaryColor   string
	SecondaryColor string
	TextColor      string
}
