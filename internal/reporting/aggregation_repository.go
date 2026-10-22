package reporting

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AggregationRepository struct {
	db *gorm.DB
}

func NewAggregationRepository(db *gorm.DB) *AggregationRepository {
	return &AggregationRepository{db: db}
}

// dailyWindow returns the half-open [begin, end) interval the daily
// rollup covers: the previous calendar day in loc.
//
// The bounds are built from the calendar fields instead of subtracting
// 24 hours from today's midnight. On a DST transition day the two
// disagree by an hour, and "midnight minus 24h" no longer lands on the
// previous local midnight.
func dailyWindow(now time.Time, loc *time.Location) (begin, end time.Time) {
	year, month, day := now.In(loc).Date()
	begin = time.Date(year, month, day-1, 0, 0, 0, 0, loc)
	end = time.Date(year, month, day, 0, 0, 0, 0, loc)
	return begin, end
}

// monthlyWindow returns the half-open [begin, end) interval the
// monthly rollup covers: the previous calendar month in loc.
func monthlyWindow(now time.Time, loc *time.Location) (begin, end time.Time) {
	year, month, _ := now.In(loc).Date()
	begin = time.Date(year, month-1, 1, 0, 0, 0, 0, loc)
	end = time.Date(year, month, 1, 0, 0, 0, 0, loc)
	return begin, end
}

// GenerateDaily rolls up the previous day's playback_logs into the
// content_stats table (one row per content_hash, period_type=day)
// and deletes the source rows so the table stays small.
//
// The whole flow runs in a single transaction so a partial write
// cannot leave playback_logs deleted but content_stats empty. Any
// panic is logged with the full stack trace and re-raised so the
// caller (the cron job) sees the failure rather than a silent
// "aggregation ran but did nothing".
func (r *AggregationRepository) GenerateDaily(ctx context.Context) (err error) {
	begin, end := dailyWindow(time.Now(), time.Local)

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if rec := recover(); rec != nil {
			slog.Default().Error("aggregation: GenerateDaily panic",
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())))
			// Best-effort rollback; the tx handle is already
			// invalid if the panic came from a connection error.
			_ = tx.Rollback().Error
			panic(rec)
		}
	}()

	var stats []struct {
		Content    string
		TotalViews int
		TimePlayed int
	}
	if err = tx.Table("playback_logs").
		Select("content, COUNT(id) as total_views, COALESCE(SUM(ended_at - started_at) / 1000, 0) as time_played").
		Group("content").
		Where("ended_at < ? AND ended_at >= ? AND ended_at > 0", end.UnixMilli(), begin.UnixMilli()).
		Find(&stats).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	if len(stats) == 0 {
		// No rows to roll up; commit the empty tx so we don't hold
		// a write transaction open for nothing.
		return tx.Commit().Error
	}

	dayStats := make([]ContentStat, 0, len(stats))
	for _, daily := range stats {
		dayStats = append(dayStats, ContentStat{
			ContentHash: daily.Content,
			PeriodType:  PeriodTypeDay,
			PeriodStart: begin,
			Seconds:     daily.TimePlayed,
			Views:       daily.TotalViews,
		})
	}

	if err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "content_hash"}, {Name: "period_type"}, {Name: "period_start"}},
		DoUpdates: clause.AssignmentColumns([]string{"seconds", "views"}),
	}).Create(&dayStats).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	if err = tx.Unscoped().Table("playback_logs").Where("ended_at < ? AND ended_at >= ? AND ended_at > 0", end.UnixMilli(), begin.UnixMilli()).Delete(nil).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	return tx.Commit().Error
}

// GenerateMonthly rolls up the previous month's per-day content_stats
// into a single per-month row per content_hash, then deletes the
// source day rows. Same panic-and-error semantics as GenerateDaily.
func (r *AggregationRepository) GenerateMonthly(ctx context.Context) (err error) {
	begin, end := monthlyWindow(time.Now(), time.Local)

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if rec := recover(); rec != nil {
			slog.Default().Error("aggregation: GenerateMonthly panic",
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())))
			_ = tx.Rollback().Error
			panic(rec)
		}
	}()

	var stats []struct {
		ContentHash string
		TotalViews  int
		TimePlayed  int
	}
	if err = tx.Model(&ContentStat{}).
		Select("content_hash, SUM(views) as total_views, SUM(seconds) as time_played").
		Group("content_hash").
		Where("period_start >= ? AND period_start < ? AND period_type = ?", begin, end, PeriodTypeDay).
		Find(&stats).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	if len(stats) == 0 {
		return tx.Commit().Error
	}

	// Clear any stale month rows from a previous run before
	// inserting the freshly aggregated values.
	if err = tx.Unscoped().
		Where("period_start >= ? AND period_start < ? AND period_type = ?", begin, end, PeriodTypeMonth).
		Delete(&ContentStat{}).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	monthStats := make([]ContentStat, 0, len(stats))
	for _, daily := range stats {
		monthStats = append(monthStats, ContentStat{
			ContentHash: daily.ContentHash,
			PeriodType:  PeriodTypeMonth,
			PeriodStart: begin,
			Seconds:     daily.TimePlayed,
			Views:       daily.TotalViews,
		})
	}

	if err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "content_hash"}, {Name: "period_type"}, {Name: "period_start"}},
		DoUpdates: clause.AssignmentColumns([]string{"seconds", "views"}),
	}).Create(&monthStats).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	if err = tx.Unscoped().
		Where("period_start >= ? AND period_start < ? AND period_type = ?", begin, end, PeriodTypeDay).
		Delete(&ContentStat{}).Error; err != nil {
		_ = tx.Rollback().Error
		return err
	}

	return tx.Commit().Error
}
