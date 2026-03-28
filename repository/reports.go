package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func SaveCrashReport(report models.CrashReport) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	if err := db.Create(&report).Error; err != nil {
		return err
	}

	return nil
}
