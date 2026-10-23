package database

import (
	"errors"
	"testing"
)

// TestIsConcurrentMigrationError covers the error texts a concurrent
// migrator produces. The MySQL/MariaDB server numbers are the ones
// the driver puts in the message; SQLite and PostgreSQL report the
// same condition as "already exists".
func TestIsConcurrentMigrationError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "duplicate key name",
			err:  errors.New("Error 1061 (42000): Duplicate key name 'idx_playback_logs_ended_at'"),
			want: true,
		},
		{
			name: "duplicate foreign key constraint name",
			err:  errors.New("Error 1826 (42000): Duplicate foreign key constraint name 'fk_events'"),
			want: true,
		},
		{
			name: "missing index on drop",
			err:  errors.New("Error 1091 (42000): Can't DROP 'idx_scraper_leagues_source_league_id'; check that column/key exists"),
			want: true,
		},
		{
			name: "deadlock",
			err:  errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction"),
			want: true,
		},
		{
			name: "lock wait timeout",
			err:  errors.New("Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction"),
			want: true,
		},
		{
			name: "duplicate column name",
			err:  errors.New("Error 1060 (42S21): Duplicate column name 'external_match_id'"),
			want: true,
		},
		{
			name: "sqlite duplicate index",
			err:  errors.New("index idx_playback_logs_ended_at already exists"),
			want: true,
		},
		{
			name: "sqlite duplicate column",
			err:  errors.New("duplicate column name: external_match_id"),
			want: true,
		},
		{
			name: "syntax error",
			err:  errors.New("Error 1064 (42000): You have an error in your SQL syntax"),
			want: false,
		},
		{
			name: "connection refused",
			err:  errors.New("dial tcp 127.0.0.1:3306: connectex: connection refused"),
			want: false,
		},
		{
			name: "no error",
			err:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isConcurrentMigrationError(tc.err); got != tc.want {
				t.Errorf("isConcurrentMigrationError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestRetryOnConcurrentMigration_RetriesConcurrentErrors makes sure a
// statement that lost the race against another migrator is run again.
func TestRetryOnConcurrentMigration_RetriesConcurrentErrors(t *testing.T) {
	attempts := 0
	err := retryOnConcurrentMigration(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("Error 1061 (42000): Duplicate key name 'idx_playback_logs_ended_at'")
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

// TestRetryOnConcurrentMigration_GivesUpOnPermanentErrors makes sure a
// real failure is reported instead of being retried until the attempts
// run out.
func TestRetryOnConcurrentMigration_GivesUpOnPermanentErrors(t *testing.T) {
	want := errors.New("table does not exist")
	attempts := 0
	err := retryOnConcurrentMigration(func() error {
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

// TestRetryOnConcurrentMigration_GivesUpAfterTheAttempts makes sure a
// race that never resolves is reported instead of hanging the boot.
func TestRetryOnConcurrentMigration_GivesUpAfterTheAttempts(t *testing.T) {
	attempts := 0
	err := retryOnConcurrentMigration(func() error {
		attempts++
		return errors.New("Error 1061 (42000): Duplicate key name 'idx_playback_logs_ended_at'")
	})
	if err == nil {
		t.Fatal("err = nil, want the duplicate key error")
	}
	if attempts != migrateRetryAttempts {
		t.Errorf("attempts = %d, want %d", attempts, migrateRetryAttempts)
	}
}
