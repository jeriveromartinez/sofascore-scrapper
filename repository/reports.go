package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func SaveCrashReport(report reporting.CrashReport) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return reporting.NewRepository(db).SaveCrash(report)
}
