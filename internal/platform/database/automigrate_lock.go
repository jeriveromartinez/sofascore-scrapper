package database

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

const (
	// migrationLockName prefixes the lock that serializes the migration
	// of every backend that boots against the same database.
	migrationLockName = "scores365:automigrate"
	// migrationBudget bounds how long the boot waits for the lock and
	// for a concurrent migration to clear, including the retries. A
	// single DDL still runs to completion under the session lock wait.
	migrationBudget = 60 * time.Second
	// migrationSessionLockWait bounds the metadata-lock and row-lock
	// waits on the migration connection. MariaDB defaults them to
	// 86400s and 50s, long enough to outlive every healthcheck.
	migrationSessionLockWait = 10 * time.Second
)

// migrationLocker is the lock lifecycle around the migration. The
// MySQL/MariaDB implementation is the only real one; tests inject a
// fake to check the ordering without a server.
type migrationLocker interface {
	configure(conn *gorm.DB, deadline time.Time) error
	reset(conn *gorm.DB)
	acquire(conn *gorm.DB, deadline time.Time) error
	release(conn *gorm.DB)
}

// withMigrationLock runs fn while holding the cluster-wide migration
// lock, so only one backend migrates at a time. MySQL and MariaDB are
// the supported servers with a cross-connection lock; other dialects
// (SQLite in the unit tests) run fn directly.
func withMigrationLock(db *gorm.DB, deadline time.Time, fn func(*gorm.DB) error) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return fn(db)
	}
	return runWithMigrationLock(db.Session(&gorm.Session{}), deadline, &mysqlMigrationLock{}, fn)
}

func runWithMigrationLock(db *gorm.DB, deadline time.Time, locker migrationLocker, fn func(*gorm.DB) error) error {
	return db.Connection(func(conn *gorm.DB) error {
		// The lock statements must not share conn's statement: GORM's
		// Connection hands back a non-cloning session, and scanning a
		// value into it leaves the connection-level statement pointing
		// at that value, which later renames the table AutoMigrate
		// targets.
		aux := conn.Session(&gorm.Session{})
		defer locker.reset(aux)
		if err := locker.configure(aux, deadline); err != nil {
			return err
		}
		if err := locker.acquire(aux, deadline); err != nil {
			return err
		}
		defer locker.release(aux)
		return fn(conn.Session(&gorm.Session{}))
	})
}

type mysqlMigrationLock struct {
	name string
}

func (l *mysqlMigrationLock) configure(conn *gorm.DB, deadline time.Time) error {
	wait := wholeSeconds(cappedWait(deadline, migrationSessionLockWait))
	err := conn.Exec(fmt.Sprintf(
		"SET SESSION lock_wait_timeout = %d, innodb_lock_wait_timeout = %d",
		wait, wait,
	)).Error
	if err != nil {
		return fmt.Errorf("configure migration session lock timeouts: %w", err)
	}
	return nil
}

// reset restores the server defaults before the pinned connection
// returns to the pool: leaving the 10s waits behind would change the
// timeout of whichever request gets that connection next.
func (l *mysqlMigrationLock) reset(conn *gorm.DB) {
	err := conn.Exec(
		"SET SESSION lock_wait_timeout = @@GLOBAL.lock_wait_timeout, " +
			"innodb_lock_wait_timeout = @@GLOBAL.innodb_lock_wait_timeout",
	).Error
	if err != nil {
		slog.Default().Warn("database: restore migration session lock timeouts",
			slog.Any("error", err))
	}
}

func (l *mysqlMigrationLock) acquire(conn *gorm.DB, deadline time.Time) error {
	var database sql.NullString
	if err := conn.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
		return fmt.Errorf("resolve the migration database: %w", err)
	}
	l.name = databaseLockName(database.String)

	wait := wholeSeconds(remaining(deadline))
	var acquired sql.NullInt64
	err := conn.Raw("SELECT GET_LOCK(?, ?)", l.name, wait).Scan(&acquired).Error
	if err != nil {
		return fmt.Errorf("acquire migration lock %q: %w", l.name, err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("acquire migration lock %q: timed out after %ds", l.name, wait)
	}
	return nil
}

func (l *mysqlMigrationLock) release(conn *gorm.DB) {
	if l.name == "" {
		return
	}
	var released sql.NullInt64
	err := conn.Raw("SELECT RELEASE_LOCK(?)", l.name).Scan(&released).Error
	if err != nil {
		slog.Default().Warn("database: release migration lock",
			slog.String("lock", l.name),
			slog.Any("error", err))
		return
	}
	if !released.Valid || released.Int64 != 1 {
		slog.Default().Warn("database: migration lock was not held at release",
			slog.String("lock", l.name))
	}
}

// databaseLockName scopes the lock to one database. GET_LOCK names are
// server-wide, so two databases on one server (staging and production,
// two developers, two CI jobs) would otherwise serialize each other.
// The name is hashed because MariaDB caps lock names at 64 characters.
func databaseLockName(database string) string {
	if database == "" {
		return migrationLockName
	}
	sum := fnv.New64a()
	_, _ = sum.Write([]byte(database))
	return fmt.Sprintf("%s:%x", migrationLockName, sum.Sum64())
}

func remaining(deadline time.Time) time.Duration {
	return time.Until(deadline)
}

func cappedWait(deadline time.Time, limit time.Duration) time.Duration {
	left := remaining(deadline)
	if left > limit {
		return limit
	}
	return left
}

func wholeSeconds(d time.Duration) int {
	seconds := int(d / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}
