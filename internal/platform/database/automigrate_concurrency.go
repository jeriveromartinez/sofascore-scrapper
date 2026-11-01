package database

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// migrationRetryBaseDelay is the first backoff step; it grows linearly
// with the attempt number and carries jitter so backends that collided
// once do not collide again on the same schedule.
const migrationRetryBaseDelay = 100 * time.Millisecond

// concurrentMigrationNumbers are the MySQL/MariaDB server errors that
// mean another backend migrated the same database at the same time.
//
// 1050 table already exists, 1060 duplicate column, 1061 duplicate
// key name, 1062 duplicate entry, 1091 index already gone, 1205 lock
// wait timeout, 1213 deadlock, 1826 duplicate foreign key, 1831
// duplicate index.
var concurrentMigrationNumbers = map[uint16]struct{}{
	1050: {},
	1060: {},
	1061: {},
	1062: {},
	1091: {},
	1205: {},
	1213: {},
	1826: {},
	1831: {},
}

// concurrentMigrationMarkers are the fallback texts for drivers that
// do not expose the server number: SQLite reports the same condition
// as "already exists" and the MySQL numbers are kept for wrapped
// errors whose type was lost.
var concurrentMigrationMarkers = []string{
	"Error 1060",
	"Error 1061",
	"Error 1062",
	"Error 1091",
	"Error 1205",
	"Error 1213",
	"Error 1826",
	"Error 1831",
	"already exists",
	"duplicate column name",
}

// isConcurrentMigrationError reports whether err is the signature of
// another backend migrating the same database at the same time.
//
// GORM's AutoMigrate is a check-then-act sequence: it asks the server
// whether an index exists and creates it when the answer is no. Two
// backends that boot together can both see "no" and both create it;
// the loser gets a duplicate-object error and the boot fails. The
// metadata locks of two concurrent DDL statements collide the same
// way.
func isConcurrentMigrationError(err error) bool {
	if err == nil {
		return false
	}
	var serverErr *mysql.MySQLError
	if errors.As(err, &serverErr) {
		if _, ok := concurrentMigrationNumbers[serverErr.Number]; ok {
			return true
		}
	}
	msg := err.Error()
	for _, marker := range concurrentMigrationMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// migrationRetry runs a migration step again when it lost the race
// against another migrator. The fields are injectable so tests never
// wait on the wall clock, and the deadline is shared by every step so
// the whole boot stays bounded.
type migrationRetry struct {
	deadline  time.Time
	baseDelay time.Duration
	sleep     func(time.Duration)
	now       func() time.Time
	log       *slog.Logger
}

// run retries fn until it succeeds, until it fails with an error a
// retry cannot clear, or until the deadline is reached.
//
// The guarded operations are idempotent, so a retry only repeats work
// another backend already committed: the check that precedes the
// failing statement now sees the object and skips it.
func (r migrationRetry) run(fn func() error) error {
	for attempt := 1; ; attempt++ {
		err := fn()
		if err == nil || !isConcurrentMigrationError(err) {
			return err
		}
		delay := r.delay(attempt)
		if r.now().Add(delay).After(r.deadline) {
			return err
		}
		r.logger().Warn("database: retrying after a concurrent migration",
			append([]any{
				slog.Int("attempt", attempt),
				slog.Duration("delay", delay),
			}, retryErrorAttrs(err)...)...)
		r.sleep(delay)
	}
}

func (r migrationRetry) delay(attempt int) time.Duration {
	base := time.Duration(attempt) * r.baseDelay
	if base <= 0 {
		return 0
	}
	return base + rand.N(r.baseDelay/2+1)
}

func (r migrationRetry) logger() *slog.Logger {
	if r.log != nil {
		return r.log
	}
	return slog.Default()
}

// retryErrorAttrs logs the server error number instead of the driver
// message: MariaDB quotes the duplicated value in a 1062 message, and
// those values can be tokens or hashes.
func retryErrorAttrs(err error) []any {
	var serverErr *mysql.MySQLError
	if errors.As(err, &serverErr) {
		return []any{slog.Uint64("code", uint64(serverErr.Number))}
	}
	return []any{slog.String("error", err.Error())}
}
