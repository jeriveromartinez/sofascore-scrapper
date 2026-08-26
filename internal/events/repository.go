package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db           *gorm.DB
	scheduleLogo func(*gorm.DB, int64, string)
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db:           db,
		scheduleLogo: func(*gorm.DB, int64, string) {},
	}
}

func NewRepositoryWithLogoScheduler(db *gorm.DB, scheduler TeamLogoScheduler) *Repository {
	repository := NewRepository(db)
	if scheduler != nil {
		repository.scheduleLogo = scheduler.Schedule
	}
	return repository
}

func (r *Repository) ReconcileTeamLogos(ctx context.Context) error {
	var teams []Team
	if err := r.db.WithContext(ctx).Find(&teams).Error; err != nil {
		return fmt.Errorf("load teams for logo reconciliation: %w", err)
	}

	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, team := range teams {
			localURL := TeamLogoAPIPath(team.TeamId)
			if team.LogoUrl == localURL {
				continue
			}
			if err := tx.Model(&Team{}).Where("team_id = ?", team.TeamId).Update("logo_url", localURL).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("normalize team logo URLs: %w", err)
	}

	for _, team := range teams {
		if _, err := os.Stat(TeamLogoLocalPath(team.TeamId)); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect logo for team %d: %w", team.TeamId, err)
		}
		r.scheduleLogo(r.db, team.TeamId, TeamLogoSourceURL(team.TeamId))
	}
	return nil
}

type pendingLogo struct {
	teamID    int64
	sourceURL string
}

func prepareTeamLogo(team *Team) pendingLogo {
	pending := pendingLogo{teamID: team.TeamId, sourceURL: team.LogoUrl}
	team.LogoUrl = TeamLogoAPIPath(team.TeamId)
	return pending
}

func (r *Repository) upsertTeam(ctx context.Context, team *Team) error {
	pending := prepareTeamLogo(team)
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "team_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "logo_url", "primary_color", "secondary_color", "text_color",
		}),
	}).Create(team).Error; err != nil {
		return err
	}
	if pending.sourceURL != "" && !isProxiedLogoURL(pending.sourceURL) {
		r.scheduleLogo(r.db, pending.teamID, pending.sourceURL)
	}
	return nil
}

func (r *Repository) Upsert(ctx context.Context, events []Event, sport string) error {
	now := time.Now().Unix()
	for i := range events {
		event := &events[i]
		event.ScrapedAt = now
		event.Sport = sport

		if event.HomeTeamModel != nil {
			if err := r.upsertTeam(ctx, event.HomeTeamModel); err != nil {
				return err
			}
		}

		if event.AwayTeamModel != nil {
			if err := r.upsertTeam(ctx, event.AwayTeamModel); err != nil {
				return err
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

func (r *Repository) UpsertScrapeBatch(ctx context.Context, batch ScrapeBatch, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 500
	}

	pendingLogos := make([]pendingLogo, 0, len(batch.Teams))
	for i := range batch.Teams {
		pending := prepareTeamLogo(&batch.Teams[i])
		if pending.sourceURL != "" && !isProxiedLogoURL(pending.sourceURL) {
			pendingLogos = append(pendingLogos, pending)
		}
	}

	now := time.Now().Unix()
	for i := range batch.Events {
		batch.Events[i].ScrapedAt = now
		batch.Events[i].HomeTeamModel = nil
		batch.Events[i].AwayTeamModel = nil
		batch.Events[i].League = nil
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(batch.Teams) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "team_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "logo_url", "primary_color", "secondary_color", "text_color",
				}),
			}).CreateInBatches(batch.Teams, batchSize).Error; err != nil {
				return err
			}
		}

		if len(batch.Tournaments) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "slug", "region",
				}),
			}).CreateInBatches(batch.Tournaments, batchSize).Error; err != nil {
				return err
			}
		}

		if len(batch.Events) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "sofa_score_event_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"sport", "home_score", "away_score",
					"home_team_id", "away_team_id",
					"start_timestamp", "current_period_start_timestamp",
					"slug", "league_id", "status_type", "scraped_at",
				}),
			}).CreateInBatches(batch.Events, batchSize).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, pending := range pendingLogos {
		r.scheduleLogo(r.db, pending.teamID, pending.sourceURL)
	}

	return nil
}

func isProxiedLogoURL(url string) bool {
	return strings.HasPrefix(url, "/teams/logo/") || strings.HasPrefix(url, "/api/app/v1/teams/logo/")
}

func downloadAndUpdateTeamLogo(ctx context.Context, db *gorm.DB, teamID int64, sourceURL string) {
	if _, err := DownloadTeamLogoWithContext(ctx, teamID, sourceURL); err != nil {
		log.Printf("events: failed to download logo for team %d: %v", teamID, err)
		return
	}
	if ctx.Err() != nil {
		return
	}

	apiPath := TeamLogoAPIPath(teamID)
	if err := db.WithContext(ctx).Model(&Team{}).Where("team_id = ?", teamID).Update("logo_url", apiPath).Error; err != nil {
		log.Printf("events: failed to update logo URL for team %d: %v", teamID, err)
	}
}

func (r *Repository) ResolveTournamentIDs(ctx context.Context, devID uint) ([]uint, error) {
	var deviceTournaments []tournaments.DeviceTournament
	if err := r.db.WithContext(ctx).Find(&deviceTournaments, "device_id = ?", devID).Error; err != nil {
		return nil, err
	}

	if len(deviceTournaments) > 0 {
		ids := make([]uint, len(deviceTournaments))
		for i, dt := range deviceTournaments {
			ids[i] = dt.TournamentID
		}
		return ids, nil
	}

	var globalConfig []tournaments.GlobalTournamentConfig
	if err := r.db.WithContext(ctx).Find(&globalConfig).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, len(globalConfig))
	for i, gc := range globalConfig {
		ids[i] = gc.TournamentID
	}
	return ids, nil
}

func (r *Repository) GetCurrentAndUpcoming(ctx context.Context, devID uint, limit int) ([]Event, error) {
	tournamentIDs, err := r.ResolveTournamentIDs(ctx, devID)
	if err != nil {
		return nil, err
	}
	return r.getCurrentAndUpcoming(ctx, tournamentIDs, limit)
}

func (r *Repository) getCurrentAndUpcoming(ctx context.Context, tournamentIDs []uint, limit int) ([]Event, error) {
	if limit <= 0 || limit > 6 {
		limit = 6
	}

	var events []Event

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

func (r *Repository) ListPage(ctx context.Context, startTimestamp int64, id uint, limit int, direction string) ([]Event, bool, error) {
	desc := strings.EqualFold(direction, "desc")
	order := "start_timestamp ASC, id ASC"
	if desc {
		order = "start_timestamp DESC, id DESC"
	}

	query := r.db.WithContext(ctx).Order(order).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League")

	if startTimestamp > 0 {
		if desc {
			query = query.Where("start_timestamp < ? OR (start_timestamp = ? AND id < ?)", startTimestamp, startTimestamp, id)
		} else {
			query = query.Where("start_timestamp > ? OR (start_timestamp = ? AND id > ?)", startTimestamp, startTimestamp, id)
		}
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
