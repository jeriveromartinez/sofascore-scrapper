package push

import (
	"context"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"gorm.io/gorm"
)

// Repository is the persistence layer for the push domain. It is
// intentionally thin: each method maps to a single SQL operation
// or transaction. Business rules (audience filter, validation,
// state transitions) live in the Service.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a Repository bound to the given GORM DB.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// InsertPushMessageWithTargets creates a push and links it to the
// given domain IDs in a single transaction. The provided now is
// used to set sent_at; pass nil to let the DB default fire.
//
// The GORM association on PushMessage.Domains is intentionally
// not used here: we go through the explicit join table so the
// delivery_attempts snapshot can rely on push_message_targets
// being authoritative.
func (r *Repository) InsertPushMessageWithTargets(ctx context.Context, m *PushMessage, domainIDs []uint, now *time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if now != nil {
			m.CreatedAt = *now
			m.UpdatedAt = *now
		}
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		if len(domainIDs) == 0 {
			return nil
		}
		rows := make([]PushMessageTarget, 0, len(domainIDs))
		for _, id := range domainIDs {
			rows = append(rows, PushMessageTarget{PushMessageID: m.ID, DomainID: id})
		}
		return tx.Create(&rows).Error
	})
}

// InsertDeliveryAttempts bulk-creates the snapshot of one row per
// (push, device). Called immediately after the audience has been
// resolved, before any device is contacted.
func (r *Repository) InsertDeliveryAttempts(ctx context.Context, attempts []DeliveryAttempt) error {
	if len(attempts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&attempts).Error
}

// MarkDeliveryDelivered flips the row to DELIVERED and stamps
// acked_at + latency_ms (now - sent_at). Lookup is by the
// transport message_id (UUID v4), which is UNIQUE on the table.
// Latency is computed in Go (not via SQL) for MariaDB/SQLite parity.
func (r *Repository) MarkDeliveryDelivered(ctx context.Context, messageID string, ackedAt time.Time) error {
	// Read sent_at first to compute latency. If sent_at is null
	// (which should never happen but is a defensive guard), skip
	// the latency update entirely.
	var existing DeliveryAttempt
	if err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		First(&existing).Error; err != nil {
		return err
	}
	updates := map[string]any{
		"state":          StateDelivered,
		"acked_at":       ackedAt,
		"failure_reason": nil,
	}
	if existing.SentAt != nil {
		latencyMS := ackedAt.Sub(*existing.SentAt).Milliseconds()
		if latencyMS < 0 {
			latencyMS = 0
		}
		updates["latency_ms"] = int(latencyMS)
	}
	return r.db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("message_id = ?", messageID).
		Updates(updates).Error
}

// MarkDeliveryFailed flips the row to FAILED and records the
// reason. Used for timeouts, ws_disconnected, domain_mismatch, and
// device_offline. No ack_at is set. Lookup is by message_id.
func (r *Repository) MarkDeliveryFailed(ctx context.Context, messageID string, reason FailureReason) error {
	return r.db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("message_id = ?", messageID).
		Updates(map[string]any{
			"state":          StateFailed,
			"failure_reason": reason,
		}).Error
}

// ListAudienceDevicesForDomains returns every device whose
// domain_id is in the given set AND is non-null. Devices without a
// domain are excluded — they cannot receive pushes (see the
// devices.Documentation for the rationale).
func (r *Repository) ListAudienceDevicesForDomains(ctx context.Context, domainIDs []uint) ([]devices.Device, error) {
	if len(domainIDs) == 0 {
		return nil, nil
	}
	var rows []devices.Device
	err := r.db.WithContext(ctx).
		Where("domain_id IN ? AND domain_id IS NOT NULL", domainIDs).
		Find(&rows).Error
	return rows, err
}

// InsertScheduledPushWithTargets creates a schedule and links it
// to the given domain IDs. For one_shot, the caller must set
// RunAt; for recurring, CronExpr.
func (r *Repository) InsertScheduledPushWithTargets(ctx context.Context, s *ScheduledPush, domainIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(s).Error; err != nil {
			return err
		}
		if len(domainIDs) == 0 {
			return nil
		}
		rows := make([]ScheduledPushTarget, 0, len(domainIDs))
		for _, id := range domainIDs {
			rows = append(rows, ScheduledPushTarget{ScheduledPushID: s.ID, DomainID: id})
		}
		return tx.Create(&rows).Error
	})
}

// ListDueScheduledPushes returns up to limit active schedules whose
// next_fire_at <= now. The runner calls this every tick (default
// 30s). Order is by next_fire_at ASC so the oldest overdue row fires
// first.
func (r *Repository) ListDueScheduledPushes(ctx context.Context, now time.Time, limit int) ([]ScheduledPush, error) {
	var rows []ScheduledPush
	err := r.db.WithContext(ctx).
		Where("is_active = ? AND next_fire_at <= ?", true, now).
		Order("next_fire_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// GetPushMessageTargets returns the join rows linking a push to
// its target domains. Callers flatten this to []uint{uint32(...)}
// when building the proto response.
func (r *Repository) GetPushMessageTargets(ctx context.Context, pushID uint) ([]PushMessageTarget, error) {
	var rows []PushMessageTarget
	err := r.db.WithContext(ctx).
		Where("push_message_id = ?", pushID).
		Find(&rows).Error
	return rows, err
}

// GetScheduledPushTargets returns the join rows linking a schedule
// to its target domains.
func (r *Repository) GetScheduledPushTargets(ctx context.Context, schedID uint) ([]ScheduledPushTarget, error) {
	var rows []ScheduledPushTarget
	err := r.db.WithContext(ctx).
		Where("scheduled_push_id = ?", schedID).
		Find(&rows).Error
	return rows, err
}

// MarkScheduledPushFired deactivates a one_shot (is_active=false)
// and stamps last_fired_at. For recurring schedules the runner
// calls RescheduleRecurring instead.
func (r *Repository) MarkScheduledPushFired(ctx context.Context, id uint, firedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"is_active":     false,
			"last_fired_at": firedAt,
		}).Error
}

// RescheduleRecurring updates next_fire_at and stamps last_fired_at
// for a recurring schedule. The runner computes the new
// next_fire_at from the cron expression and passes it in.
func (r *Repository) RescheduleRecurring(ctx context.Context, id uint, nextFireAt, firedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"next_fire_at":  nextFireAt,
			"last_fired_at": firedAt,
		}).Error
}

// DeactivateAllForUser sets is_active=false on every active
// schedule for the user. Called when the user toggles
// notifications_enabled off so any pending firings do not run.
func (r *Repository) DeactivateAllForUser(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Update("is_active", false).Error
}

// GetPushMessageByID fetches a single push. Returns
// ErrNotFound-equivalent (gorm.ErrRecordNotFound) when the row does
// not exist OR the user_id does not match (the latter prevents
// enumeration of other users' pushes). Target domains are fetched
// separately via GetPushMessageTargets.
func (r *Repository) GetPushMessageByID(ctx context.Context, id, userID uint) (*PushMessage, error) {
	var m PushMessage
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListPushMessagesByUser returns the user's most recent pushes,
// optionally filtered by source ("immediate" or "scheduled"). The
// rows are returned in created_at DESC order. Pagination is
// cursor-based: pass the last id from a previous page to fetch the
// next N older rows. hasMore=true means there is at least one more
// row beyond the returned slice. Target domains are fetched
// separately via GetPushMessageTargets.
func (r *Repository) ListPushMessagesByUser(ctx context.Context, userID uint, source string, limit int) ([]PushMessage, bool, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC")
	if source == "immediate" || source == "scheduled" {
		q = q.Where("source = ?", source)
	}
	var rows []PushMessage
	if err := q.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

// ListScheduledPushesByUser is the schedule equivalent of
// ListPushMessagesByUser. is_active rows are kept (the dashboard
// shows deactivated ones with a "paused" badge). Target domains
// are fetched separately via GetScheduledPushTargets.
func (r *Repository) ListScheduledPushesByUser(ctx context.Context, userID uint, limit int) ([]ScheduledPush, bool, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC")
	var rows []ScheduledPush
	if err := q.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

// GetScheduledPushByID fetches a single schedule. Same ownership
// guard as GetPushMessageByID: a row owned by a different user
// returns ErrRecordNotFound. Target domains are fetched separately
// via GetScheduledPushTargets.
func (r *Repository) GetScheduledPushByID(ctx context.Context, id, userID uint) (*ScheduledPush, error) {
	var s ScheduledPush
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateScheduledPushActive flips is_active. The only mutable field
// after create; cron and run_at are immutable to keep the
// next_fire_at invariant sane. To change the schedule, the user
// deletes and re-creates.
func (r *Repository) UpdateScheduledPushActive(ctx context.Context, id, userID uint, isActive bool) (*ScheduledPush, error) {
	res := r.db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_active", isActive)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.GetScheduledPushByID(ctx, id, userID)
}

// DeleteScheduledPush soft-deletes a schedule by setting
// is_active=false. The row stays in the table for audit; the
// runner simply skips inactive rows.
func (r *Repository) DeleteScheduledPush(ctx context.Context, id, userID uint) error {
	res := r.db.WithContext(ctx).Model(&ScheduledPush{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AggregateMetricsForUser returns the per-user snapshot that the
// dashboard renders. The spec calls for several counters
// (delivered_total, delivery_rate, active_schedules, fires_24h,
// etc.) and a few breakdowns (top_platforms, top_app_versions,
// hourly_histogram_30d). The full implementation lives in
// metrics_aggregator.go; this thin wrapper just hands off the call.
func (r *Repository) AggregateMetricsForUser(ctx context.Context, userID uint) (*pb.PushMetricsAggregate, error) {
	return buildAggregateSnapshot(ctx, r.db, userID)
}

// CampaignMetrics returns the per-campaign snapshot for the given
// push id, scoped to the user. Computed from delivery_attempts.
func (r *Repository) CampaignMetrics(ctx context.Context, id, userID uint) (*pb.PushMetricsByCampaign, error) {
	return buildCampaignSnapshot(ctx, r.db, id, userID)
}
