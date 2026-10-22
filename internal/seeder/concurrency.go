package seeder

import (
	"strings"
	"time"
)

// isTransientLockError reports whether err is a MySQL/MariaDB locking
// failure (deadlock or lock wait timeout) that a retry can clear.
// The server error numbers are stable; the driver error text carries
// them as "Error 1213" and "Error 1205".
func isTransientLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1213") || strings.Contains(msg, "Error 1205")
}

// retryOnLockConflict runs fn until it succeeds, until it fails with
// an error a retry cannot clear, or until the attempts are used up.
// The guarded operations are idempotent, so a retry only repeats work
// that has already been committed.
func retryOnLockConflict(attempts int, fn func() error) error {
	var err error
	for attempt := range attempts {
		err = fn()
		if err == nil || !isTransientLockError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return err
}
