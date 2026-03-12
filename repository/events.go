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
