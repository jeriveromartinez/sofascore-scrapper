package models

import "gorm.io/gorm"

type Tournament struct {
	gorm.Model
	Name string `json:"name"`
}
