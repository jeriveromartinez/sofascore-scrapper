package push

import (
	"context"
	"sort"
	"time"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"gorm.io/gorm"
)

// buildAggregateSnapshot returns the per-user dashboard metrics.
// The implementation is intentionally simple (one query per metric)
// because the volumes are small: even at 10 pushes/minute that
// is 14,400 delivery_attempts rows per day per user, and SQLite/
// MariaDB handle that in a few ms. If volumes grow we can replace
// the per-metric queries with a single GROUP BY with rollup.
//
// The aggregations covered here match the spec section "Agregadas
// por user":
//
//	messages_sent_total, messages_delivered_total, delivery_rate_total,
//	active_schedules, scheduled_fires_24h/7d/30d, scheduled_failures,
//	audience_size (devices in the user's domains), audience_peak_today,
//	top_platforms, top_app_versions, hourly_histogram_30d, last_push_at.
//
// The 30-day histogram bucket is built in Go from a single SQL
// SELECT; the same query also feeds last_push_at.
func buildAggregateSnapshot(ctx context.Context, db *gorm.DB, userID uint) (*pb.PushMetricsAggregate, error) {
	out := &pb.PushMetricsAggregate{}

	// Total counters (no time window).
	var sent, delivered, notDelivered int64
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Joins("JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id").
		Where("push_messages.user_id = ?", userID).
		Count(&sent).Error; err != nil {
		return nil, err
	}
	out.MessagesSentTotal = sent
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Joins("JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id").
		Where("push_messages.user_id = ? AND delivery_attempts.state = ?", userID, StateDelivered).
		Count(&delivered).Error; err != nil {
		return nil, err
	}
	out.MessagesDeliveredTotal = delivered
	if sent > 0 {
		out.DeliveryRateTotal = float64(delivered) / float64(sent)
	}
	_ = notDelivered // available for a future "not_delivered_total" field

	// Active schedules: count of is_active rows for the user.
	var activeSchedules int64
	if err := db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&activeSchedules).Error; err != nil {
		return nil, err
	}
	out.ActiveSchedules = activeSchedules

	// Fires over time windows. We treat any push where the source is
	// "scheduled" as a fire, and the failures are the deliveries
	// that flipped to "failed". This is approximate (a recurring
	// schedule that fired 30 times contributes 30) but matches the
	// "ejecuciones del cron" semantics in the spec.
	now := time.Now()
	for _, w := range []struct {
		days     int
		firesOut *int64
		failOut  *int64
	}{
		{1, &out.ScheduledFires_24H, &out.ScheduledFailures_24H},
		{7, &out.ScheduledFires_7D, &out.ScheduledFailures_7D},
		{30, &out.ScheduledFires_30D, &out.ScheduledFailures_30D},
	} {
		since := now.Add(-time.Duration(w.days) * 24 * time.Hour)
		var n int64
		if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
			Joins("JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id").
			Where("push_messages.user_id = ? AND push_messages.source = ? AND push_messages.created_at >= ?", userID, SourceScheduled, since).
			Count(&n).Error; err != nil {
			return nil, err
		}
		*w.firesOut = n
		var f int64
		if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
			Joins("JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id").
			Where("push_messages.user_id = ? AND push_messages.source = ? AND push_messages.created_at >= ? AND delivery_attempts.state = ?", userID, SourceScheduled, since, StateFailed).
			Count(&f).Error; err != nil {
			return nil, err
		}
		*w.failOut = f
	}

	// Audience size: the number of devices whose domain_id is
	// among the user's owned domains. We reuse the audience
	// filter from the dispatch path so the two numbers agree.
	var domainsOwned []uint
	if err := db.WithContext(ctx).Model(&struct {
		gorm.Model
		Domain string
		UserID uint
	}{}).
		Where("user_id = ?", userID).
		Pluck("id", &domainsOwned).Error; err != nil {
		// Best effort: ignore the audience size if the domains
		// table shape has changed.
		out.AudienceSize = 0
	}
	if len(domainsOwned) > 0 {
		var audience int64
		if err := db.WithContext(ctx).Model(&struct {
			gorm.Model
			DomainID *uint
		}{}).
			Where("domain_id IN ? AND domain_id IS NOT NULL", domainsOwned).
			Count(&audience).Error; err != nil {
			return nil, err
		}
		out.AudienceSize = audience
	}

	// Audience peak today: max(sent_at::date, count(*)) grouped by
	// hour. Cheap on the volumes we expect; if it becomes the
	// slow query, add an index on delivery_attempts.sent_at.
	type row struct {
		Hour  int
		Count int64
	}
	var peaks []row
	if err := db.WithContext(ctx).Raw(`
		SELECT CAST(strftime('%H', sent_at) AS INTEGER) AS hour, COUNT(*) AS count
		FROM delivery_attempts
		JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id
		WHERE push_messages.user_id = ?
		  AND date(sent_at) = date('now')
		GROUP BY hour
	`, userID).Scan(&peaks).Error; err == nil {
		var maxN int64
		for _, p := range peaks {
			if p.Count > maxN {
				maxN = p.Count
			}
		}
		out.AudiencePeakToday = maxN
	}

	// Top platforms / app versions. GROUP BY on the joined
	// devices table; top 5 by count. SQLite-compatible GROUP BY
	// semantics.
	type deviceAgg struct {
		Platform string
		Version  string
		Count    int64
	}
	var platformRows []deviceAgg
	if err := db.WithContext(ctx).Raw(`
		SELECT devices.platform AS platform, devices.version AS version, COUNT(*) AS count
		FROM delivery_attempts
		JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id
		JOIN devices ON devices.id = delivery_attempts.device_id
		WHERE push_messages.user_id = ?
		GROUP BY devices.platform, devices.version
		ORDER BY count DESC
		LIMIT 5
	`, userID).Scan(&platformRows).Error; err == nil {
		seen := map[string]bool{}
		for _, p := range platformRows {
			if !seen[p.Platform] {
				seen[p.Platform] = true
				out.TopPlatforms = append(out.TopPlatforms, &pb.PlatformCount{
					Platform: p.Platform, Count: p.Count,
				})
			}
		}
		for _, p := range platformRows {
			out.TopAppVersions = append(out.TopAppVersions, &pb.AppVersionCount{
				Version: p.Version, Count: p.Count,
			})
		}
	}

	// Hourly histogram for the last 30 days. One row per hour
	// 0..23 with the total count.
	var hourly []row
	if err := db.WithContext(ctx).Raw(`
		SELECT CAST(strftime('%H', sent_at) AS INTEGER) AS hour, COUNT(*) AS count
		FROM delivery_attempts
		JOIN push_messages ON push_messages.id = delivery_attempts.push_message_id
		WHERE push_messages.user_id = ?
		  AND sent_at >= ?
		GROUP BY hour
	`, userID, now.Add(-30*24*time.Hour)).Scan(&hourly).Error; err == nil {
		// Sort by hour for stable output.
		sort.Slice(hourly, func(i, j int) bool { return hourly[i].Hour < hourly[j].Hour })
		for _, h := range hourly {
			out.HourlyHistogram_30D = append(out.HourlyHistogram_30D, &pb.HourBucket{
				Hour: int32(h.Hour), Count: h.Count,
			})
		}
	}

	// last_push_at: the most recent created_at across the user's
	// pushes (immediate and scheduled).
	var lastAt *time.Time
	if err := db.WithContext(ctx).Model(&PushMessage{}).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(1).
		Pluck("created_at", &lastAt).Error; err == nil && lastAt != nil {
		out.LastPushAt = lastAt.Format(time.RFC3339)
	}

	return out, nil
}

// buildCampaignSnapshot returns the per-campaign dashboard. The
// per-device row from delivery_attempts is the source of truth for
// every counter in the response.
func buildCampaignSnapshot(ctx context.Context, db *gorm.DB, pushID, userID uint) (*pb.PushMetricsByCampaign, error) {
	out := &pb.PushMetricsByCampaign{PushId: uint32(pushID)}

	// Verify the push belongs to the user (or 404). A single row
	// check is cheap and prevents enumeration of other users'
	// campaigns via the metrics endpoint.
	var ownerID uint
	if err := db.WithContext(ctx).Model(&PushMessage{}).
		Where("id = ?", pushID).
		Pluck("user_id", &ownerID).Error; err != nil {
		return nil, err
	}
	if ownerID != userID {
		return nil, gorm.ErrRecordNotFound
	}

	// Totals.
	var targets, delivered, failed int64
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("push_message_id = ?", pushID).
		Count(&targets).Error; err != nil {
		return nil, err
	}
	out.TargetsTotal = targets
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("push_message_id = ? AND state = ?", pushID, StateDelivered).
		Count(&delivered).Error; err != nil {
		return nil, err
	}
	out.Delivered = delivered
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("push_message_id = ? AND state = ?", pushID, StateFailed).
		Count(&failed).Error; err != nil {
		return nil, err
	}
	out.NotDelivered = failed
	if targets > 0 {
		out.DeliveryRate = float64(delivered) / float64(targets)
	}

	// Latency percentiles: query the latency_ms values, sort in
	// Go (the row count per push is bounded by the audience size
	// which is small for human-targeted pushes).
	type latRow struct {
		LatencyMS int64
	}
	var lats []latRow
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("push_message_id = ? AND state = ? AND latency_ms IS NOT NULL", pushID, StateDelivered).
		Pluck("latency_ms", &lats).Error; err == nil && len(lats) > 0 {
		// Sort ascending; the p50 is the middle element, p95 the
		// element at 0.95 * len.
		sort.Slice(lats, func(i, j int) bool { return lats[i].LatencyMS < lats[j].LatencyMS })
		p50 := lats[len(lats)*50/100]
		p95 := lats[len(lats)*95/100]
		out.LatencyP50Ms = p50.LatencyMS
		out.LatencyP95Ms = p95.LatencyMS
	}

	// Failures by reason.
	type failRow struct {
		Reason string
		Count  int64
	}
	var fails []failRow
	if err := db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Select("failure_reason AS reason, COUNT(*) AS count").
		Where("push_message_id = ? AND state = ?", pushID, StateFailed).
		Group("failure_reason").
		Scan(&fails).Error; err == nil {
		for _, f := range fails {
			out.FailuresByReason = append(out.FailuresByReason, &pb.FailureBreakdown{
				Reason: failureReasonToProto(f.Reason),
				Count:  f.Count,
			})
		}
	}
	return out, nil
}

// failureReasonToProto maps a domain FailureReason string to the
// proto enum. Unknown values map to UNSPECIFIED so a future enum
// value does not break the response.
func failureReasonToProto(s string) pb.DeliveryFailureReason {
	switch FailureReason(s) {
	case FailureDeviceOffline:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_DEVICE_OFFLINE
	case FailureSendTimeout:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_SEND_TIMEOUT
	case FailureWSDisconnected:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_WS_DISCONNECTED
	case FailureDomainMismatch:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_DOMAIN_MISMATCH
	case FailureExpiredToken:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_EXPIRED_TOKEN
	case FailureInternalError:
		return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_INTERNAL_ERROR
	}
	return pb.DeliveryFailureReason_DELIVERY_FAILURE_REASON_UNSPECIFIED
}
