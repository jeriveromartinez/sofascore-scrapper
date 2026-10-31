package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
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
// abstraction any search-by-name logo source satisfies so the
// downloader can fall back to it when the ID-derived CDN URL returns
// 404 for a team_id. No implementation is currently
// wired in production; the interface is kept for future sources.
type LogoLookup interface {
	TeamLogoURL(ctx context.Context, name string) (string, error)
}

type Repository struct {
	db           *gorm.DB
	scheduleLogo func(*gorm.DB, LogoJob)
	logoLookup   LogoLookup
	// idSourceURL builds the last-resort, ID-derived badge URL. It
	// defaults to TeamLogoSourceURL and is only swapped out by tests,
	// which would otherwise have to reach the live image CDNs to
	// exercise the tail of the source chain.
	idSourceURL func(int64) string
	// truncationWarned records the devices already reported as having
	// their tournament assignment truncated, so an oversized assignment
	// does not warn on every request for the life of the process.
	truncationWarned sync.Map
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db:           db,
		scheduleLogo: func(*gorm.DB, LogoJob) {},
		idSourceURL:  TeamLogoSourceURL,
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

const reconcileTeamLogosBatchSize = 500

// teamLogoAPIPathSQL is the SQL equivalent of TeamLogoAPIPath for the
// team_id column. sqlite has no CONCAT and MariaDB only treats || as
// concatenation under PIPES_AS_CONCAT, so the expression is per dialect.
func teamLogoAPIPathSQL(db *gorm.DB) clause.Expr {
	if db.Dialector.Name() == "sqlite" {
		return gorm.Expr("? || CAST(team_id AS TEXT)", teamLogoAPIPrefix)
	}
	return gorm.Expr("CONCAT(?, team_id)", teamLogoAPIPrefix)
}

func (r *Repository) ReconcileTeamLogos(ctx context.Context) error {
	db := r.db.WithContext(ctx)
	localURL := teamLogoAPIPathSQL(db)
	if err := db.Model(&Team{}).
		Where("logo_url IS NULL OR logo_url <> ?", localURL).
		Update("logo_url", localURL).Error; err != nil {
		return fmt.Errorf("normalize team logo URLs: %w", err)
	}

	var batch []Team
	var inspectErr error
	err := db.Select("id", "team_id", "name").
		FindInBatches(&batch, reconcileTeamLogosBatchSize, func(_ *gorm.DB, _ int) error {
			for _, team := range batch {
				if _, err := os.Stat(TeamLogoLocalPath(team.TeamId)); err == nil {
					continue
				} else if !errors.Is(err, os.ErrNotExist) {
					inspectErr = fmt.Errorf("inspect logo for team %d: %w", team.TeamId, err)
					return inspectErr
				}
				r.scheduleLogo(r.db, LogoJob{
					TeamID:     team.TeamId,
					TeamName:   team.Name,
					PrimaryURL: TeamLogoSourceURL(team.TeamId),
				})
			}
			return nil
		}).Error
	if inspectErr != nil {
		return inspectErr
	}
	if err != nil {
		return fmt.Errorf("load teams for logo reconciliation: %w", err)
	}
	return nil
}

type pendingLogo = LogoJob

func prepareTeamLogo(team *Team) LogoJob {
	pending := LogoJob{TeamID: team.TeamId, TeamName: team.Name, PrimaryURL: team.LogoUrl}
	team.LogoUrl = TeamLogoAPIPath(team.TeamId)
	return pending
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
// LogoLookup, and the ID-derived CDN fallback — and updates
// Team.logo_url to the proxied API path on the first hit.
//
// The chain order matters:
//  1. Primary URL (scraper-provided, e.g. the competitor badge that
//     came back on the 365scores feed).
//  2. LogoLookup by name — only consulted when the primary fails and
//     a LogoLookup has been installed via WithLogoLookup.
//  3. The ID-derived CDN URL (TeamLogoSourceURL) — last resort, and
//     only reachable for teams whose ID namespace still has an
//     upstream that serves badges by ID.
//
// The lookup step is rate-limited by the upstream source, so we only
// consult it after the primary has already failed. This keeps the
// common path (primary succeeds) cost-free.
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
			slog.Default().WarnContext(ctx, "events: logo lookup failed",
				slog.Int64("team_id", job.TeamID),
				slog.String("team_name", job.TeamName),
				slog.String("err", lookupErr.Error()),
			)
		} else if url != "" && r.tryDownloadAndPersist(ctx, db, job.TeamID, url) {
			return
		}
	}
	if r.tryDownloadAndPersist(ctx, db, job.TeamID, r.idSource(job.TeamID)) {
		return
	}
	slog.Default().WarnContext(ctx, "events: no logo source succeeded",
		slog.Int64("team_id", job.TeamID),
		slog.String("team_name", job.TeamName),
		slog.String("primary_url", job.PrimaryURL),
	)
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
		slog.Default().WarnContext(ctx, "events: failed to download logo",
			slog.Int64("team_id", teamID),
			slog.String("url", url),
			slog.String("err", err.Error()),
		)
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
		slog.Default().WarnContext(ctx, "events: failed to update logo URL",
			slog.Int64("team_id", teamID),
			slog.String("err", err.Error()),
		)
	}
	return true
}

// maxTournamentIDs bounds the league-id slice fed to the
// WHERE league_id IN (?) clause in getCurrentAndUpcoming. The
// planner switches strategies as the IN-list grows (MySQL gives
// up on index usage beyond a few hundred entries), so an
// unbounded device assignment combined with the full global
// fallback produces an O(N) plan that scans the whole events
// table.
//
// 200 is well above any real device assignment today (~10s of
// leagues is the largest expected) and keeps the query in the
// range where the planner uses the league_id index. Devices that
// genuinely need more fall back to the global list (which is
// what they would have seen before this cap existed) — the
// truncation is documented in the log so operators can spot it.
const maxTournamentIDs = 200

func (r *Repository) ResolveTournamentIDs(ctx context.Context, devID uint) ([]uint, error) {
	var deviceTournaments []tournaments.DeviceTournament
	if err := r.db.WithContext(ctx).
		Order("tournament_id ASC").
		Find(&deviceTournaments, "device_id = ?", devID).Error; err != nil {
		return nil, err
	}

	if len(deviceTournaments) > 0 {
		ids := make([]uint, len(deviceTournaments))
		for i, dt := range deviceTournaments {
			ids[i] = dt.TournamentID
		}
		return r.truncateTournamentIDs(devID, ids), nil
	}

	var globalConfig []tournaments.GlobalTournamentConfig
	if err := r.db.WithContext(ctx).
		Order("tournament_id ASC").
		Find(&globalConfig).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, len(globalConfig))
	for i, gc := range globalConfig {
		ids[i] = gc.TournamentID
	}
	return r.truncateTournamentIDs(devID, ids), nil
}

// truncateTournamentIDs caps the returned slice at maxTournamentIDs.
// The caller (getCurrentAndUpcoming) treats a truncated list as
// 'best-effort subset of the device's leagues' — the same behaviour
// the user would get by manually trimming the assignment. The ids are
// ordered by the caller so truncation always keeps the same lowest
// ids, and the warning is emitted once per device per process.
func (r *Repository) truncateTournamentIDs(devID uint, ids []uint) []uint {
	if len(ids) <= maxTournamentIDs {
		return ids
	}
	if _, alreadyWarned := r.truncationWarned.LoadOrStore(devID, struct{}{}); !alreadyWarned {
		slog.Default().Warn("events: truncating device tournament assignments to keep the IN-list query plan in index-scan range",
			slog.Uint64("device_id", uint64(devID)),
			slog.Int("assignments", len(ids)),
			slog.Int("cap", maxTournamentIDs),
		)
	}
	return ids[:maxTournamentIDs]
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

	// ResolveTournamentIDs returns an empty (non-nil) slice for a device
	// with no device_tournaments and an empty global config. Building
	// `WHERE league_id IN ()` is a syntax error in MariaDB/MySQL/Postgres,
	// so short-circuit here instead of producing a 500 to the caller.
	if len(tournamentIDs) == 0 {
		return nil, nil
	}

	var events []Event

	// The join drops events whose league was soft-deleted: Preload would
	// otherwise return them with a nil League. Every column is qualified
	// because tournaments has an id column too, which makes a bare id
	// ambiguous in ORDER BY and in the id sub-filter below.
	activeLeagueJoin := "JOIN tournaments ON tournaments.id = events.league_id AND tournaments.deleted_at IS NULL"

	if err := r.db.WithContext(ctx).Joins(activeLeagueJoin).
		Where("events.status_type = ? AND events.league_id IN ?", "inprogress", tournamentIDs).
		Order("events.start_timestamp DESC, events.id DESC").
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
		query := r.db.WithContext(ctx).Joins(activeLeagueJoin).
			Where("events.status_type = ? AND events.start_timestamp >= ? AND events.league_id IN ?", "notstarted", nowMs, tournamentIDs).
			Order("events.start_timestamp ASC, events.id ASC")
		if len(existingIDs) > 0 {
			query = query.Where("events.id NOT IN ?", existingIDs)
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
	// The soft-delete guard lives in the ON clause so the name filters
	// agree with the Preloads, which never return deleted rows.
	if f.LeagueName != "" {
		query = query.
			Joins("LEFT JOIN tournaments AS leagues ON leagues.id = events.league_id AND leagues.deleted_at IS NULL").
			Where("leagues.name LIKE ? ESCAPE '\\'", "%"+escapeLike(f.LeagueName)+"%")
	}
	if f.TeamName != "" {
		query = query.
			Joins("LEFT JOIN teams AS home_team_models ON home_team_models.team_id = events.home_team_id AND home_team_models.deleted_at IS NULL").
			Joins("LEFT JOIN teams AS away_team_models ON away_team_models.team_id = events.away_team_id AND away_team_models.deleted_at IS NULL").
			Where(
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

// idSource resolves the last link of the logo source chain. A
// Repository built by NewRepository always carries a builder; the
// nil guard keeps a zero-value Repository (or one restored from an
// older constructor) from panicking mid-download.
func (r *Repository) idSource(teamID int64) string {
	if r.idSourceURL == nil {
		return TeamLogoSourceURL(teamID)
	}
	return r.idSourceURL(teamID)
}
