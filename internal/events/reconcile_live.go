package events

import (
	"context"
	"time"
)

// maxLiveEventAge is the oldest a match can plausibly still be in play.
// The reconciler uses it so a long match is never closed just because a
// scrape was missed.
const maxLiveEventAge = 6 * time.Hour

// CloseStaleLive finishes live events the scraper stopped refreshing
// once they are also older than any plausible match duration. The feed
// only reports matches that exist, so an event that silently drops out
// keeps its last "inprogress" status forever; without this job the table
// accumulates zombies. The API filters stale live events on read, so a
// missed reconciliation run cannot surface them to the app.
func (r *Repository) CloseStaleLive(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&Event{}).
		Where("status_type = ? AND scraped_at < ? AND start_timestamp < ?",
			"inprogress",
			now.Add(-liveEventFreshness).Unix(),
			now.Add(-maxLiveEventAge).UnixMilli()).
		Update("status_type", "finished")
	return result.RowsAffected, result.Error
}
