package auth

import (
	"context"
	"sync"
	"time"
)

// RefreshTokenCleanupInterval is the minimum delay between two
// purges of the refresh_tokens table.
const RefreshTokenCleanupInterval = time.Hour

// RefreshTokenCleanup purges refresh tokens that are past their
// expiry. Those rows can never be used again — RotateRefreshToken
// rejects them — but nothing deleted them, so the table grew
// forever.
type RefreshTokenCleanup struct {
	repo *AuthRepository

	mu      sync.Mutex
	lastRun time.Time
	now     func() time.Time
}

func NewRefreshTokenCleanup(repo *AuthRepository) *RefreshTokenCleanup {
	return &RefreshTokenCleanup{repo: repo, now: time.Now}
}

// Run deletes every refresh token that expired before now and
// returns the number of deleted rows.
func (c *RefreshTokenCleanup) Run(ctx context.Context) (int64, error) {
	if c == nil || c.repo == nil {
		return 0, nil
	}
	deleted, err := c.repo.PurgeExpiredRefreshTokens(ctx, c.now())
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	c.lastRun = c.now()
	c.mu.Unlock()
	return deleted, nil
}

// RunIfDue runs the purge only when the last one is older than
// RefreshTokenCleanupInterval. It reports whether the purge ran.
func (c *RefreshTokenCleanup) RunIfDue(ctx context.Context) (int64, error) {
	if c == nil || c.repo == nil {
		return 0, nil
	}
	c.mu.Lock()
	last := c.lastRun
	c.mu.Unlock()

	if !last.IsZero() && c.now().Sub(last) < RefreshTokenCleanupInterval {
		return 0, nil
	}
	return c.Run(ctx)
}
