package database

import (
	"strings"
	"time"
)

// migrateRetryAttempts is how many times a migration statement is
// retried before the boot fails.
const migrateRetryAttempts = 5

// migrateRetryDelay is the base delay between two attempts. It grows
// linearly with the attempt number.
const migrateRetryDelay = 100 * time.Millisecond

// concurrentMigrationMarkers are the error texts that mean another
// backend migrated the same database at the same time.
//
// The MySQL/MariaDB server error numbers are stable and the driver
// carries them in the error text. SQLite and PostgreSQL report the
// same condition as "already exists".
var concurrentMigrationMarkers = []string{
	"Error 1061", // duplicate key name
	"Error 1826", // duplicate foreign key constraint name
	"Error 1091", // can't drop; check that column/key exists
	"Error 1213", // deadlock found
	"Error 1205", // lock wait timeout exceeded
	"already exists",
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
	msg := err.Error()
	for _, marker := range concurrentMigrationMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// retryOnConcurrentMigration runs fn until it succeeds, until it fails
// with an error a retry cannot clear, or until the attempts are used
// up.
//
// The guarded operations are idempotent, so a retry only repeats work
// another backend already committed: the check that precedes the
// failing statement now sees the object and skips it.
func retryOnConcurrentMigration(fn func() error) error {
	var err error
	for attempt := range migrateRetryAttempts {
		err = fn()
		if err == nil || !isConcurrentMigrationError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * migrateRetryDelay)
	}
	return err
}
