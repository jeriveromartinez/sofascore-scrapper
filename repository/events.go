package repository

import (
	"log"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/imageproxy"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var downloadSem = make(chan struct{}, 10)

func SaveSofaScoreEvent(Events []*models.APIEvent, sport string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}

	now := time.Now().Unix()
	for _, event := range Events {
		model := event.ToSofaScoreEvent()

		homeTeam := event.HomeTeam.ToSofaScoreTeam()
		db.FirstOrCreate(&homeTeam, models.Team{TeamId: model.HomeTeamId})
		if !isProxiedLogoURL(homeTeam.LogoUrl) {
			scheduleLogoDownload(db, model.HomeTeamId, homeTeam.LogoUrl)
		}

		awayTeam := event.AwayTeam.ToSofaScoreTeam()
		db.FirstOrCreate(&awayTeam, models.Team{TeamId: model.AwayTeamId})
		if !isProxiedLogoURL(awayTeam.LogoUrl) {
			scheduleLogoDownload(db, model.AwayTeamId, awayTeam.LogoUrl)
		}

		tournament := models.Tournament{Slug: event.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(event.Tournament.UniqueTournament.Category.Slug), Name: event.Tournament.UniqueTournament.Name, Region: event.Tournament.UniqueTournament.Category.Name, Model: gorm.Model{ID: uint(event.Tournament.UniqueTournament.ID)}}
		db.FirstOrCreate(&tournament, models.Tournament{Slug: event.Tournament.UniqueTournament.Slug + "-" + strings.ToLower(event.Tournament.UniqueTournament.Category.Slug)})

		model.ScrapedAt = now
		model.Sport = sport
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sofa_score_event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"home_score", "away_score", "current_period_start_timestamp", "scraped_at"}),
		}).Create(&model)
	}
}

func isProxiedLogoURL(url string) bool {
	return strings.HasPrefix(url, "/api/v1/teams/logo/")
}

func scheduleLogoDownload(db *gorm.DB, teamID int64, sourceURL string) {
	select {
	case downloadSem <- struct{}{}:
		go func() {
			defer func() { <-downloadSem }()
			downloadAndUpdateTeamLogo(db.Session(&gorm.Session{}), teamID, sourceURL)
		}()
	default:
		// Skip this round if downloader is saturated; next scrape will retry.
	}
}

func downloadAndUpdateTeamLogo(db *gorm.DB, teamID int64, sourceURL string) {
	if _, err := imageproxy.DownloadTeamLogo(teamID, sourceURL); err != nil {
		log.Printf("repository: failed to download logo for team %d: %v", teamID, err)
		return
	}

	apiPath := imageproxy.TeamLogoAPIPath(teamID)
	if err := db.Model(&models.Team{}).Where("team_id = ?", teamID).Update("logo_url", apiPath).Error; err != nil {
		log.Printf("repository: failed to update logo URL for team %d: %v", teamID, err)
	}
}

func GetCurrentAndUpcomingEvents(devId uint, limit int) ([]models.SofaScoreEvent, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 6 {
		limit = 6
	}

	now := time.Now().Add(-(time.Minute * 5)).Unix()
	var events []models.SofaScoreEvent
	var selfEvents []models.DeviceTournament

	var tournamentIDs []uint
	db.Find(&selfEvents, "device_id = ?", devId)
	if len(selfEvents) > 0 {
		tournamentIDs = make([]uint, len(selfEvents))
		for i, dt := range selfEvents {
			tournamentIDs[i] = dt.TournamentID
		}
	} else {
		var globalConfig []models.GlobalTournamentConfig
		db.Find(&globalConfig)
		tournamentIDs = make([]uint, len(globalConfig))
		for i, gc := range globalConfig {
			tournamentIDs[i] = gc.TournamentID
		}
	}

	db.Where("current_period_start_timestamp >= ? AND league_id IN ?", now, tournamentIDs).
		Order("current_period_start_timestamp DESC").
		Limit(limit).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League").
		Find(&events)

	if len(events) < limit {
		remaining := limit - len(events)
		var upcomingEvents []models.SofaScoreEvent
		existingIDs := make([]uint, len(events))
		for i, e := range events {
			existingIDs[i] = e.ID
		}

		now = time.Now().Add((time.Minute * 5)).Unix()
		query := db.Where("start_timestamp > ? AND league_id IN ?", now, tournamentIDs).Order("start_timestamp ASC")
		if len(existingIDs) > 0 {
			query = query.Where("id NOT IN ?", existingIDs)
		}

		query.Limit(remaining).
			Preload("HomeTeamModel").
			Preload("AwayTeamModel").
			Preload("League").
			Find(&upcomingEvents)

		events = append(events, upcomingEvents...)
	}

	return events, nil
}
