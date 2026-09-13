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

// EventsPageFilter holds all query parameters for ListPage. The zero value
// (with Direction="", Limit=0, etc.) is not valid; callers must set Limit and
// Direction explicitly.
type EventsPageFilter struct {
	CursorStartTimestamp int64
	CursorID             uint
	Limit                int
	Direction            string
	FromTimestampMs      int64
	Sport                string
	Status               string
	LeagueName           string
	TeamName             string
}

// escapeLike escapes the three special characters recognised by SQL LIKE
// (% multi-char wildcard, _ single-char wildcard, \ literal escape char) by
// prefixing each with the LIKE escape character (backslash). This prevents a
// caller from constructing a wildcard query from arbitrary user input, per
// spec §6.1 / §10 (LIKE wildcard injection).
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// LogoLookup resolves a team name to a remote logo URL. It is the
// abstraction TheSportsDB satisfies so the downloader can fall back
// to a search-by-name source when the SofaScore CDN returns 404 for
// a FotMob-derived team_id.
type LogoLookup interface {
	TeamLogoURL(ctx context.Context, name string) (string, error)
}

type Repository struct {
	db           *gorm.DB
	scheduleLogo func(*gorm.DB, LogoJob)
	logoLookup   LogoLookup
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db:           db,
		scheduleLogo: func(*gorm.DB, LogoJob) {},
	}
}

// WithLogoLookup installs a fallback source resolver used by
// downloadAndUpdateTeamLogo when the primary URL fails. Pass nil to
// restore the no-fallback behaviour (useful in tests).
func (r *Repository) WithLogoLookup(lookup LogoLookup) *Repository {
	r.logoLookup = lookup
	return r
}

// WithLogoScheduler installs the scheduler used by ReconcileTeamLogos
// (and any internal caller) to enqueue per-team download jobs. Pass
// nil to restore the no-op default.
//
// The scheduler is intentionally separate from the handler closure
// (DownloadAndPersistLogo): NewLogoScheduler takes the handler, this
// method takes the queue. Together they form a full backfill loop —
// the scheduler's worker pool drains jobs, and ReconcileTeamLogos
// keeps enqueuing missing teams on each stack restart.
func (r *Repository) WithLogoScheduler(scheduler TeamLogoScheduler) *Repository {
	if scheduler == nil {
		r.scheduleLogo = func(*gorm.DB, LogoJob) {}
		return r
	}
	r.scheduleLogo = scheduler.Schedule
	return r
}

// DB exposes the underlying *gorm.DB so callers that still need raw
// access (e.g. admin handlers using offset-based queries that have not
// been promoted to repository methods) can share the same connection
// pool as the repository. Prefer adding a new method on Repository
// over reaching for this getter; the goal is to delete every direct
// query, not to keep it alive forever.
func (r *Repository) DB() *gorm.DB {
	return r.db
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
		r.scheduleLogo(r.db, LogoJob{
			TeamID:     team.TeamId,
			TeamName:   team.Name,
			PrimaryURL: TeamLogoSourceURL(team.TeamId),
		})
	}
	return nil
}

type pendingLogo = LogoJob

func prepareTeamLogo(team *Team) LogoJob {
	pending := LogoJob{TeamID: team.TeamId, TeamName: team.Name, PrimaryURL: team.LogoUrl}
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
	if pending.PrimaryURL != "" && !isProxiedLogoURL(pending.PrimaryURL) {
		r.scheduleLogo(r.db, pending)
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
		if pending.PrimaryURL != "" && !isProxiedLogoURL(pending.PrimaryURL) {
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
				Columns: []clause.Column{{Name: "external_match_id"}},
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
		r.scheduleLogo(r.db, pending)
	}

	return nil
}

func isProxiedLogoURL(url string) bool {
	return strings.HasPrefix(url, "/teams/logo/") || strings.HasPrefix(url, "/api/app/v1/teams/logo/")
}

// DownloadAndPersistLogo is the entry point the LogoScheduler calls
// for every job. It walks the source chain — primary URL, optional
// TheSportsDB lookup, and the ID-based SofaScore CDN fallback — and
// updates Team.logo_url to the proxied API path on the first hit.
//
// The chain order matters:
//  1. Primary URL (scraper-provided, e.g. from FotMob CDN).
//  2. TheSportsDB by name — only consulted when the primary fails and
//     a LogoLookup has been installed via WithLogoLookup.
//  3. SofaScore CDN by team_id — last resort, only works when FotMob
//     and SofaScore share an ID for the team (true for ~25% of the
//     teams we currently seed).
//
// The lookup step is rate-limited by TheSportsDB's free public key
// (~30 req/min), so we only consult it after the primary has
// already failed. This keeps the common path (primary succeeds)
// cost-free.
//
// All errors are logged but never propagated; a single team's
// download failure must not stop the scheduler.
//
// db may be nil (used by tests that exercise the source chain
// without a backing store). When the download succeeds and db is
// nil, the LogoUrl update is silently skipped — the file on disk
// still serves the logo through the API endpoint.
func (r *Repository) DownloadAndPersistLogo(ctx context.Context, db *gorm.DB, job LogoJob) {
	if r.tryDownloadAndPersist(ctx, db, job.TeamID, job.PrimaryURL) {
		return
	}
	if r.logoLookup != nil && strings.TrimSpace(job.TeamName) != "" {
		url, lookupErr := r.logoLookup.TeamLogoURL(ctx, job.TeamName)
		if lookupErr != nil {
			log.Printf("events: logo lookup for team %d (%q) failed: %v", job.TeamID, job.TeamName, lookupErr)
		} else if url != "" && r.tryDownloadAndPersist(ctx, db, job.TeamID, url) {
			return
		}
	}
	if r.tryDownloadAndPersist(ctx, db, job.TeamID, TeamLogoSourceURL(job.TeamID)) {
		return
	}
	log.Printf("events: no logo source succeeded for team %d (name=%q, primary=%q)", job.TeamID, job.TeamName, job.PrimaryURL)
}

// tryDownloadAndPersist attempts one source URL: writes the image
// to disk and updates Team.logo_url on success. Returns true when
// the file landed on disk — callers use this to decide whether to
// fall through to the next source. Errors are logged but never
// returned because each caller wants to continue the chain.
func (r *Repository) tryDownloadAndPersist(ctx context.Context, db *gorm.DB, teamID int64, url string) bool {
	if url == "" {
		return false
	}
	if _, err := DownloadTeamLogoWithContext(ctx, teamID, url); err != nil {
		log.Printf("events: failed to download logo for team %d from %s: %v", teamID, url, err)
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	if db == nil {
		// Test-mode: the file landed on disk so the API endpoint
		// will still serve it. The LogoUrl column update requires a
		// real *gorm.DB.
		return true
	}
	apiPath := TeamLogoAPIPath(teamID)
	if err := db.WithContext(ctx).Model(&Team{}).Where("team_id = ?", teamID).Update("logo_url", apiPath).Error; err != nil {
		log.Printf("events: failed to update logo URL for team %d: %v", teamID, err)
	}
	return true
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

func (r *Repository) ListPage(ctx context.Context, f EventsPageFilter) ([]Event, bool, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Direction == "" {
		f.Direction = "asc"
	}
	desc := strings.EqualFold(f.Direction, "desc")
	order := "events.start_timestamp ASC, events.id ASC"
	if desc {
		order = "events.start_timestamp DESC, events.id DESC"
	}

	query := r.db.WithContext(ctx).
		Joins("LEFT JOIN tournaments AS leagues ON leagues.id = events.league_id").
		Joins("LEFT JOIN teams AS home_team_models ON home_team_models.team_id = events.home_team_id").
		Joins("LEFT JOIN teams AS away_team_models ON away_team_models.team_id = events.away_team_id").
		Order(order).
		Preload("HomeTeamModel").
		Preload("AwayTeamModel").
		Preload("League")

	if f.CursorStartTimestamp > 0 {
		if desc {
			query = query.Where("events.start_timestamp < ? OR (events.start_timestamp = ? AND events.id < ?)",
				f.CursorStartTimestamp, f.CursorStartTimestamp, f.CursorID)
		} else {
			query = query.Where("events.start_timestamp > ? OR (events.start_timestamp = ? AND events.id > ?)",
				f.CursorStartTimestamp, f.CursorStartTimestamp, f.CursorID)
		}
	}

	if f.FromTimestampMs > 0 {
		query = query.Where("events.start_timestamp >= ?", f.FromTimestampMs)
	}
	if f.Sport != "" {
		query = query.Where("events.sport = ?", f.Sport)
	}
	if f.Status != "" {
		query = query.Where("events.status_type = ?", f.Status)
	}
	if f.LeagueName != "" {
		query = query.Where("leagues.name LIKE ? ESCAPE '\\'", "%"+escapeLike(f.LeagueName)+"%")
	}
	if f.TeamName != "" {
		query = query.Where(
			"(home_team_models.name LIKE ? ESCAPE '\\' OR away_team_models.name LIKE ? ESCAPE '\\')",
			"%"+escapeLike(f.TeamName)+"%",
			"%"+escapeLike(f.TeamName)+"%",
		)
	}

	var rows []Event
	err := query.Limit(f.Limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > f.Limit
	if hasMore {
		rows = rows[:f.Limit]
	}
	return rows, hasMore, nil
}
