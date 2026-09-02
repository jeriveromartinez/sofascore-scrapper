package tournaments

import "gorm.io/gorm"

type Tournament struct {
	gorm.Model
	Name   string `gorm:"column:name;type:longtext" json:"name"`
	Slug   string `gorm:"column:slug;type:longtext" json:"slug"`
	Region string `gorm:"column:region;type:longtext" json:"region"`
}
