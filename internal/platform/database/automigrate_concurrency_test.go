package database

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// fakeClock advances only when the retry sleeps, so the tests never
// wait on the wall clock.
type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(d time.Duration) {
	c.slept += d
	c.now = c.now.Add(d)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// TestIsConcurrentMigrationError covers the error shapes a concurrent
// migrator produces. The MySQL driver exposes the server number, so
// the check reads it from the typed error and falls back to the text
// for dialects that only report "already exists".
func TestIsConcurrentMigrationError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "duplicate key name",
			err:  &mysql.MySQLError{Number: 1061, Message: "Duplicate key name 'idx_playback_logs_ended_at'"},
			want: true,
		},
		{
			name: "duplicate foreign key constraint name",
			err:  &mysql.MySQLError{Number: 1826, Message: "Duplicate foreign key constraint name 'fk_events'"},
			want: true,
		},
		{
			name: "missing index on drop",
			err:  &mysql.MySQLError{Number: 1091, Message: "Can't DROP 'idx_scraper_leagues_source_league'"},
			want: true,
		},
		{
			name: "deadlock",
			err:  &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"},
			want: true,
		},
		{
			name: "lock wait timeout",
			err:  &mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"},
			want: true,
		},
		{
			name: "table already exists",
			err:  &mysql.MySQLError{Number: 1050, Message: "Table 'events' already exists"},
			want: true,
		},
		{
			name: "duplicate column name",
			err:  &mysql.MySQLError{Number: 1060, Message: "Duplicate column name 'external_match_id'"},
			want: true,
		},
		{
			name: "duplicate entry",
			err:  &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'fotmob-47' for key 'idx_scraper_leagues_source_league'"},
			want: true,
		},
		{
			name: "duplicate index",
			err:  &mysql.MySQLError{Number: 1831, Message: "Duplicate index 'idx_scraper_leagues_source_league'"},
			want: true,
		},
		{
			name: "wrapped driver error",
			err:  fmt.Errorf("automigrate PlaybackLog (index 9): %w", &mysql.MySQLError{Number: 1061, Message: "Duplicate key name"}),
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
			err:  &mysql.MySQLError{Number: 1064, Message: "You have an error in your SQL syntax"},
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

// TestMigrationRetry_RetriesConcurrentErrors makes sure a statement
// that lost the race against another migrator is run again.
func TestMigrationRetry_RetriesConcurrentErrors(t *testing.T) {
	clock := newFakeClock()
	retry := migrationRetry{
		deadline:  clock.Now().Add(time.Minute),
		baseDelay: 10 * time.Millisecond,
		sleep:     clock.Sleep,
		now:       clock.Now,
		log:       discardLogger(),
	}
	attempts := 0
	err := retry.run(func() error {
		attempts++
		if attempts < 3 {
			return &mysql.MySQLError{Number: 1061, Message: "Duplicate key name"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if clock.slept == 0 {
		t.Error("the retry did not wait between attempts")
	}
}

// TestMigrationRetry_GivesUpOnPermanentErrors makes sure a real failure
// is reported instead of being retried.
func TestMigrationRetry_GivesUpOnPermanentErrors(t *testing.T) {
	clock := newFakeClock()
	retry := migrationRetry{
		deadline:  clock.Now().Add(time.Minute),
		baseDelay: 10 * time.Millisecond,
		sleep:     clock.Sleep,
		now:       clock.Now,
		log:       discardLogger(),
	}
	want := errors.New("table does not exist")
	attempts := 0
	err := retry.run(func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (a permanent error must not be retried)", attempts)
	}
	if clock.slept != 0 {
		t.Errorf("slept = %s, want 0", clock.slept)
	}
}

// TestMigrationRetry_StopsAtTheDeadline makes sure a race that never
// resolves is reported instead of hanging the boot.
func TestMigrationRetry_StopsAtTheDeadline(t *testing.T) {
	clock := newFakeClock()
	deadline := clock.Now().Add(500 * time.Millisecond)
	retry := migrationRetry{
		deadline:  deadline,
		baseDelay: 100 * time.Millisecond,
		sleep:     clock.Sleep,
		now:       clock.Now,
		log:       discardLogger(),
	}
	want := &mysql.MySQLError{Number: 1061, Message: "Duplicate key name"}
	attempts := 0
	err := retry.run(func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if attempts < 2 {
		t.Errorf("attempts = %d, want at least 2", attempts)
	}
	if clock.now.After(deadline) {
		t.Errorf("the retry slept past the deadline: now = %s, deadline = %s", clock.now, deadline)
	}
}

// TestMigrationRetry_LogsEachRetry keeps the operator able to tell a
// clean boot from a race that backed off.
func TestMigrationRetry_LogsEachRetry(t *testing.T) {
	var logs bytes.Buffer
	clock := newFakeClock()
	retry := migrationRetry{
		deadline:  clock.Now().Add(time.Minute),
		baseDelay: time.Millisecond,
		sleep:     clock.Sleep,
		now:       clock.Now,
		log:       slog.New(slog.NewTextHandler(&logs, nil)),
	}
	attempts := 0
	if err := retry.run(func() error {
		attempts++
		if attempts < 2 {
			return &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}
		}
		return nil
	}); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !bytes.Contains(logs.Bytes(), []byte("concurrent migration")) {
		t.Errorf("the retry was not logged, got %q", logs.String())
	}
	if bytes.Contains(logs.Bytes(), []byte("Deadlock found")) {
		t.Errorf("the log leaked the driver message, got %q", logs.String())
	}
	if !bytes.Contains(logs.Bytes(), []byte("code=1213")) {
		t.Errorf("the log did not carry the server error number, got %q", logs.String())
	}
}
