package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"gorm.io/gorm/clause"
)

func SaveSofaScoreEvent(Events []*models.APIEvent, sport string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}

	now := time.Now().Unix()
	for _, event := range Events {
		model := event.ToSofaScoreEvent()
		model.ScrapedAt = now
		model.Sport = sport
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sofa_score_event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"home_score", "away_score", "current_period_start_timestamp", "scraped_at"}),
		}).Create(&model)

		team := models.Team{TeamId: model.HomeTeamId, LogoUrl: model.GetHomeTeamLogo()}
		db.FirstOrCreate(&team, models.Team{TeamId: model.HomeTeamId})
		team = models.Team{TeamId: model.AwayTeamId, LogoUrl: model.GetAwayTeamLogo()}
		db.FirstOrCreate(&team, models.Team{TeamId: model.AwayTeamId})
	}
}
