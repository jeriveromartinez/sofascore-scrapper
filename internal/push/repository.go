package push

import (
	"context"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
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
// acked_at + latency_ms (now - sent_at). It is called from the WS
// ack handler when the client echoes the message_id.
//
// Latency is computed in Go (not via SQL) so the same code path
// works on both MariaDB (production) and SQLite (tests). The
// tradeoff is one extra read of sent_at; the row is indexed by
// (push_message_id, device_id) so the lookup is cheap.
func (r *Repository) MarkDeliveryDelivered(ctx context.Context, pushID, deviceID uint, ackedAt time.Time) error {
	// Read sent_at first to compute latency. If sent_at is null
	// (which should never happen but is a defensive guard), skip
	// the latency update entirely.
	var existing DeliveryAttempt
	if err := r.db.WithContext(ctx).
		Where("push_message_id = ? AND device_id = ?", pushID, deviceID).
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
		Where("push_message_id = ? AND device_id = ?", pushID, deviceID).
		Updates(updates).Error
}

// MarkDeliveryFailed flips the row to FAILED and records the
// reason. Used for timeouts, ws_disconnected, domain_mismatch, and
// device_offline. No ack_at is set.
func (r *Repository) MarkDeliveryFailed(ctx context.Context, pushID, deviceID uint, reason FailureReason) error {
	return r.db.WithContext(ctx).Model(&DeliveryAttempt{}).
		Where("push_message_id = ? AND device_id = ?", pushID, deviceID).
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
		Preload("Domains").
		Where("is_active = ? AND next_fire_at <= ?", true, now).
		Order("next_fire_at ASC").
		Limit(limit).
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
