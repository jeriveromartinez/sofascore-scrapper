package events

import (
	"context"
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

func (r *Repository) Upsert(ctx context.Context, events []Event, sport string) error {
	now := time.Now().Unix()
	for i := range events {
		event := &events[i]
		event.ScrapedAt = now
		event.Sport = sport

		if event.HomeTeamModel != nil {
			r.db.WithContext(nil).FirstOrCreate(event.HomeTeamModel, Team{TeamId: event.HomeTeamModel.TeamId})
			if !isProxiedLogoURL(event.HomeTeamModel.LogoUrl) {
				scheduleLogoDownload(r.db, event.HomeTeamModel.TeamId, event.HomeTeamModel.LogoUrl)
			}
		}

		if event.AwayTeamModel != nil {
			r.db.WithContext(nil).FirstOrCreate(event.AwayTeamModel, Team{TeamId: event.AwayTeamModel.TeamId})
			if !isProxiedLogoURL(event.AwayTeamModel.LogoUrl) {
				scheduleLogoDownload(r.db, event.AwayTeamModel.TeamId, event.AwayTeamModel.LogoUrl)
			}
		}

		if event.League != nil {
			r.db.WithContext(nil).FirstOrCreate(event.League, tournaments.Tournament{Slug: event.League.Slug})
		}

		result := r.db.WithContext(nil).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sofa_score_event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"sport", "home_score", "away_score",
				"home_team_id", "away_team_id",
				"start_timestamp", "current_period_start_timestamp",
				"slug", "league_id", "status_type", "scraped_at",
			}),
		}).Create(event)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
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

func (r *Repository) GetCurrentAndUpcoming(ctx context.Context, devID uint, limit int) ([]Event, error) {
	if limit <= 0 || limit > 6 {
		limit = 6
	}

	var events []Event

	var deviceTournaments []tournaments.DeviceTournament
	if err := r.db.WithContext(ctx).Find(&deviceTournaments, "device_id = ?", devID).Error; err != nil {
		return nil, err
	}

	var tournamentIDs []uint
	if len(deviceTournaments) > 0 {
		tournamentIDs = make([]uint, len(deviceTournaments))
		for i, dt := range deviceTournaments {
			tournamentIDs[i] = dt.TournamentID
		}
	} else {
		var globalConfig []tournaments.GlobalTournamentConfig
		if err := r.db.WithContext(ctx).Find(&globalConfig).Error; err != nil {
			return nil, err
		}
		tournamentIDs = make([]uint, len(globalConfig))
		for i, gc := range globalConfig {
			tournamentIDs[i] = gc.TournamentID
		}
	}

	if err := r.db.WithContext(ctx).Where("status_type = ? AND league_id IN ?", "inprogress", tournamentIDs).
		Order("current_period_start_timestamp DESC").
		Limit(limit).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League").
		Find(&events).Error; err != nil {
		return nil, err
	}

	if len(events) < limit {
		remaining := limit - len(events)
		var upcoming []Event
		existingIDs := make([]uint, len(events))
		for i, e := range events {
			existingIDs[i] = e.ID
		}

		nowMs := time.Now().UnixMilli()
		query := r.db.WithContext(ctx).Where("status_type = ? AND start_timestamp >= ? AND league_id IN ?", "notstarted", nowMs, tournamentIDs).Order("start_timestamp ASC")
		if len(existingIDs) > 0 {
			query = query.Where("id NOT IN ?", existingIDs)
		}

		if err := query.Limit(remaining).
			Preload("HomeTeamModel").
			Preload("AwayTeamModel").
			Preload("League").
			Find(&upcoming).Error; err != nil {
			return nil, err
		}

		events = append(events, upcoming...)
	}

	return events, nil
}

func (r *Repository) ListPage(ctx context.Context, startTimestamp int64, id uint, limit int) ([]Event, bool, error) {
	query := r.db.WithContext(ctx).Order("start_timestamp ASC, id ASC").
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League")

	if startTimestamp > 0 {
		query = query.Where("start_timestamp > ? OR (start_timestamp = ? AND id > ?)", startTimestamp, startTimestamp, id)
	}

	var rows []Event
	err := query.Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

