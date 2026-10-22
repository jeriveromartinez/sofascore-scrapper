package seeder

import (
	"errors"
	"testing"
)

// TestRetryOnLockConflict_RetriesTransientErrors covers the retry of
// the MariaDB locking errors (deadlock, lock wait timeout) that
// concurrent first boots produce.
func TestRetryOnLockConflict_RetriesTransientErrors(t *testing.T) {
	attempts := 0
	err := retryOnLockConflict(3, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

// TestRetryOnLockConflict_GivesUpOnPermanentErrors makes sure a real
// failure is reported instead of being retried until the attempts run
// out.
func TestRetryOnLockConflict_GivesUpOnPermanentErrors(t *testing.T) {
	want := errors.New("table does not exist")
	attempts := 0
	err := retryOnLockConflict(3, func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (a permanent error must not be retried)", attempts)
	}
}
