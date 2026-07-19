package reporting

import (
	"time"

	"gorm.io/gorm"
)

type AggregationRepository struct {
	db *gorm.DB
}

func NewAggregationRepository(db *gorm.DB) *AggregationRepository {
	return &AggregationRepository{db: db}
}

func (r *AggregationRepository) GenerateDaily() error {
	db := r.db
	now := time.Now()
	begin := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Add(-time.Hour * 24)
	end := begin.AddDate(0, 0, 1).Add(-time.Second)

	var stats []struct {
		Content    string
		TotalViews int
		TimePlayed int
	}
	ctx := db.Begin()
	if ctx.Error != nil {
		return ctx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			ctx.Rollback()
		}
	}()

	if err := ctx.Table("playback_logs").
		Select("content, COUNT(id) as total_views, COALESCE(SUM(CAST(ended_at AS SIGNED) - CAST(started_at AS SIGNED)) DIV 1000, 0) as time_played").
		Group("content").
		Where("ended_at <= ? AND ended_at >= ? AND ended_at > 0", end.UnixMilli(), begin.UnixMilli()).
		Find(&stats).Error; err != nil {
		ctx.Rollback()
		return err
	}

	if len(stats) == 0 {
		ctx.Rollback()
		return nil
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

	if err := ctx.Save(&dayStats).Error; err != nil {
		ctx.Rollback()
		return err
	}

	if err := ctx.Unscoped().Table("playback_logs").Where("ended_at <= ? AND ended_at >= ? AND ended_at > 0", end.UnixMilli(), begin.UnixMilli()).Delete(nil).Error; err != nil {
		ctx.Rollback()
		return err
	}

	return ctx.Commit().Error
}

func (r *AggregationRepository) GenerateMonthly() error {
	db := r.db
	now := time.Now()
	begin := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	end := begin.AddDate(0, 1, 0).Add(-time.Second)
	var stats []struct {
		ContentHash string
		TotalViews  int
		TimePlayed  int
	}
	ctx := db.Begin()
	if ctx.Error != nil {
		return ctx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			ctx.Rollback()
		}
	}()

	if err := ctx.Model(&ContentStat{}).
		Select("content_hash, SUM(views) as total_views, SUM(seconds) as time_played").
		Group("content_hash").
		Where("created_at >= ? AND created_at <= ? AND period_type = ?", begin, end, PeriodTypeDay).
		Find(&stats).Error; err != nil {
		ctx.Rollback()
		return err
	}

	if len(stats) == 0 {
		ctx.Rollback()
		return nil
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

	if err := ctx.Save(&monthStats).Error; err != nil {
		ctx.Rollback()
		return err
	}

	if err := ctx.Unscoped().Delete(&ContentStat{}, "created_at >= ? AND created_at <= ? AND period_type = ?", begin, end, PeriodTypeDay).Error; err != nil {
		ctx.Rollback()
		return err
	}

	return ctx.Commit().Error
}
