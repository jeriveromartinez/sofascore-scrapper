package models

import "gorm.io/gorm"

type Tournament struct {
	gorm.Model
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Region string `json:"region"`
}
