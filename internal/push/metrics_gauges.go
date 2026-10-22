package push

import (
	"context"

	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
)

// CountEnabledUsers returns the number of users with the push
// notifications toggle on. Soft-deleted users are excluded.
func (r *Repository) CountEnabledUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&users.User{}).
		Where("notifications_enabled = ?", true).
		Count(&n).Error
	return n, err
}

// CountActiveSchedules returns the number of scheduled pushes that are
// still enabled. Soft-deleted schedules are excluded.
func (r *Repository) CountActiveSchedules(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&ScheduledPush{}).
		Where("is_active = ?", true).
		Count(&n).Error
	return n, err
}

// RefreshGauges recomputes the process-wide push gauges from the
// database and publishes them. The counts are global, so every
// instance refreshes its own registry from the shared database rather
// than relying on a single elected writer.
func (s *Service) RefreshGauges(ctx context.Context) error {
	if s == nil || s.repo == nil || s.metrics == nil {
		return nil
	}
	enabled, err := s.repo.CountEnabledUsers(ctx)
	if err != nil {
		return err
	}
	active, err := s.repo.CountActiveSchedules(ctx)
	if err != nil {
		return err
	}
	s.metrics.SetEnabledUsers(enabled)
	s.metrics.SetActiveSchedules(active)
	return nil
}
