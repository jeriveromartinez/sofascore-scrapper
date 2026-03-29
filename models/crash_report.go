package models

import "gorm.io/gorm"

type AppReport struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	Environment string `json:"environment"`
	Platform    string `json:"platform"`
}

type DeviceReport struct {
	OsVersion string `json:"osVersion"`
	Locale    string `json:"locale"`
}

type CrashReport struct {
	gorm.Model
	Fatal      bool         `json:"fatal"`
	Error      string       `json:"error"`
	StackTrace string       `json:"stackTrace"`
	Context    string       `json:"context"`
	App        AppReport    `json:"app" gorm:"embedded"`
	Device     DeviceReport `json:"device" gorm:"embedded"`
}
