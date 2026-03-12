package repository

import (
	"log"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/imageproxy"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// downloadSem caps the number of concurrent logo download goroutines to avoid
// exhausting system resources when a large batch of events is processed.
var downloadSem = make(chan struct{}, 10)

func SaveSofaScoreEvent(Events []*models.APIEvent, sport string) {
	db, err := database.GetDB()
	if err != nil {
		return
	}

	now := time.Now().Unix()
	for _, event := range Events {
		model := event.ToSofaScoreEvent()

		homeTeam := models.Team{TeamId: model.HomeTeamId, LogoUrl: model.GetHomeTeamLogo()}
		db.FirstOrCreate(&homeTeam, models.Team{TeamId: model.HomeTeamId})
		if !isProxiedLogoURL(homeTeam.LogoUrl) {
			scheduleLogoDownload(db, model.HomeTeamId, model.GetHomeTeamLogo())
		}

		awayTeam := models.Team{TeamId: model.AwayTeamId, LogoUrl: model.GetAwayTeamLogo()}
		db.FirstOrCreate(&awayTeam, models.Team{TeamId: model.AwayTeamId})
		if !isProxiedLogoURL(awayTeam.LogoUrl) {
			scheduleLogoDownload(db, model.AwayTeamId, model.GetAwayTeamLogo())
		}

		tournament := models.Tournament{Slug: event.Tournament.UniqueTournament.Slug, Name: event.Tournament.UniqueTournament.Name, Model: gorm.Model{ID: uint(event.Tournament.UniqueTournament.ID)}}
		db.FirstOrCreate(&tournament, models.Tournament{Slug: event.Tournament.UniqueTournament.Slug})

		model.ScrapedAt = now
		model.Sport = sport
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sofa_score_event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"home_score", "away_score", "current_period_start_timestamp", "scraped_at"}),
		}).Create(&model)
	}
}

// isProxiedLogoURL reports whether url already points to the local image proxy.
func isProxiedLogoURL(url string) bool {
	return strings.HasPrefix(url, "/api/v1/teams/logo/")
}

// scheduleLogoDownload acquires a slot from the semaphore and starts a
// goroutine that downloads the logo and updates the database.  Each goroutine
// uses its own GORM session to avoid sharing state across goroutines.
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

// downloadAndUpdateTeamLogo downloads the logo for the given team and, on
// success, updates the team's LogoUrl in the database to the local API path so
// that subsequent event responses return the proxied URL.
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

// GetCurrentAndUpcomingEvents retrieves up to 6 events that are currently happening
// (based on CurrentPeriodStartTimestamp) or upcoming (based on StartTimestamp)
func GetCurrentAndUpcomingEvents(limit int) ([]models.SofaScoreEvent, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 6 {
		limit = 6
	}

	now := time.Now().Add(-(time.Minute * 5)).Unix()
	var events []models.SofaScoreEvent

	// First, try to get current events (where CurrentPeriodStartTimestamp is set and recent)
	// Events are considered "current" if their CurrentPeriodStartTimestamp is within the last 3 hours
	db.Where("current_period_start_timestamp > 0 AND current_period_start_timestamp >= ?", now).
		Order("current_period_start_timestamp DESC").
		Limit(limit).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League").
		Find(&events)

	// If we don't have enough current events, fill with upcoming events
	if len(events) < limit {
		remaining := limit - len(events)
		var upcomingEvents []models.SofaScoreEvent

		// Get IDs of events we already have to exclude them
		existingIDs := make([]uint, len(events))
		for i, e := range events {
			existingIDs[i] = e.ID
		}

		now = time.Now().Add((time.Minute * 5)).Unix()
		query := db.Where("start_timestamp > ?", now).Order("start_timestamp ASC")
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
