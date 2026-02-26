package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func SaveSofaScoreEvent(Events []*models.APIEvent) {
	db, err := database.GetDB()
	if err != nil {
		return
	}

	now:= time.Now().Unix()
	for _, event := range Events {
		model := event.ToSofaScoreEvent()
		model.ScrapedAt = now
		db.Create(&model)
	}
}
