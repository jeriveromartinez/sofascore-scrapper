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

	now := time.Now().Unix()
	for _, event := range Events {
		model := event.ToSofaScoreEvent()
		model.ScrapedAt = now
		db.Create(&model)

		team := models.Team{TeamId: model.HomeTeamId, LogoUrl: model.GetHomeTeamLogo()}
		db.FirstOrCreate(&team, models.Team{TeamId: model.HomeTeamId})
		team = models.Team{TeamId: model.AwayTeamId, LogoUrl: model.GetAwayTeamLogo()}
		db.FirstOrCreate(&team, models.Team{TeamId: model.AwayTeamId})
	}
}
