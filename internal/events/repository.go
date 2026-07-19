package events

import (
	"log"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var downloadSem = make(chan struct{}, 10)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Save(events []Event, sport string) {
	now := time.Now().Unix()
	for i := range events {
		event := &events[i]
		event.ScrapedAt = now
		event.Sport = sport

		if event.HomeTeamModel != nil {
			r.db.FirstOrCreate(event.HomeTeamModel, Team{TeamId: event.HomeTeamModel.TeamId})
			if !isProxiedLogoURL(event.HomeTeamModel.LogoUrl) {
				scheduleLogoDownload(r.db, event.HomeTeamModel.TeamId, event.HomeTeamModel.LogoUrl)
			}
		}

		if event.AwayTeamModel != nil {
			r.db.FirstOrCreate(event.AwayTeamModel, Team{TeamId: event.AwayTeamModel.TeamId})
			if !isProxiedLogoURL(event.AwayTeamModel.LogoUrl) {
				scheduleLogoDownload(r.db, event.AwayTeamModel.TeamId, event.AwayTeamModel.LogoUrl)
			}
		}

		if event.League != nil {
			r.db.FirstOrCreate(event.League, tournaments.Tournament{Slug: event.League.Slug})
		}

		r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sofa_score_event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"home_score", "away_score", "current_period_start_timestamp", "scraped_at"}),
		}).Create(event)
	}
}

func isProxiedLogoURL(url string) bool {
	return strings.HasPrefix(url, "/teams/logo/")
}

func scheduleLogoDownload(db *gorm.DB, teamID int64, sourceURL string) {
	select {
	case downloadSem <- struct{}{}:
		go func() {
			defer func() { <-downloadSem }()
			downloadAndUpdateTeamLogo(db.Session(&gorm.Session{}), teamID, sourceURL)
		}()
	default:
	}
}

func downloadAndUpdateTeamLogo(db *gorm.DB, teamID int64, sourceURL string) {
	if _, err := DownloadTeamLogo(teamID, sourceURL); err != nil {
		log.Printf("events: failed to download logo for team %d: %v", teamID, err)
		return
	}

	apiPath := TeamLogoAPIPath(teamID)
	if err := db.Model(&Team{}).Where("team_id = ?", teamID).Update("logo_url", apiPath).Error; err != nil {
		log.Printf("events: failed to update logo URL for team %d: %v", teamID, err)
	}
}

func (r *Repository) GetCurrentAndUpcoming(devID uint, limit int) ([]Event, error) {
	if limit <= 0 || limit > 6 {
		limit = 6
	}

	now := time.Now().Add(-(time.Minute * 5)).Unix()
	var events []Event

	var deviceTournaments []tournaments.DeviceTournament
	r.db.Find(&deviceTournaments, "device_id = ?", devID)

	var tournamentIDs []uint
	if len(deviceTournaments) > 0 {
		tournamentIDs = make([]uint, len(deviceTournaments))
		for i, dt := range deviceTournaments {
			tournamentIDs[i] = dt.TournamentID
		}
	} else {
		var globalConfig []tournaments.GlobalTournamentConfig
		r.db.Find(&globalConfig)
		tournamentIDs = make([]uint, len(globalConfig))
		for i, gc := range globalConfig {
			tournamentIDs[i] = gc.TournamentID
		}
	}

	r.db.Where("current_period_start_timestamp >= ? AND league_id IN ?", now, tournamentIDs).
		Order("current_period_start_timestamp DESC").
		Limit(limit).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League").
		Find(&events)

	if len(events) < limit {
		remaining := limit - len(events)
		var upcoming []Event
		existingIDs := make([]uint, len(events))
		for i, e := range events {
			existingIDs[i] = e.ID
		}

		now = time.Now().Add((time.Minute * 5)).Unix()
		query := r.db.Where("start_timestamp > ? AND league_id IN ?", now, tournamentIDs).Order("start_timestamp ASC")
		if len(existingIDs) > 0 {
			query = query.Where("id NOT IN ?", existingIDs)
		}

		query.Limit(remaining).
			Preload("HomeTeamModel").
			Preload("AwayTeamModel").
			Preload("League").
			Find(&upcoming)

		events = append(events, upcoming...)
	}

	return events, nil
}

