package reporting

import "gorm.io/gorm"

type AppReport struct {
	Name        string `gorm:"type:longtext" json:"name"`
	Version     string `gorm:"type:longtext" json:"version"`
	Build       string `gorm:"type:longtext" json:"build"`
	Environment string `gorm:"type:longtext" json:"environment"`
	Platform    string `gorm:"type:longtext" json:"platform"`
}

type DeviceReport struct {
	OsVersion string `gorm:"type:longtext" json:"osVersion"`
	Locale    string `gorm:"type:longtext" json:"locale"`
}

type CrashReport struct {
	gorm.Model
	Fatal      bool         `gorm:"column:fatal" json:"fatal"`
	Error      string       `gorm:"column:error;type:longtext" json:"error"`
	StackTrace string       `gorm:"column:stack_trace;type:longtext" json:"stackTrace"`
	Context    string       `gorm:"column:context;type:longtext" json:"context"`
	App        AppReport    `json:"app" gorm:"embedded;embeddedPrefix:app_"`
	Device     DeviceReport `json:"device" gorm:"embedded;embeddedPrefix:device_"`
}
