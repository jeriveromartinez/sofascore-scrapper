package database

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

// TestWithMigrationLock_RunsWithoutADatabaseServer keeps the SQLite
// path (the one the rest of the unit tests use) working: only MySQL
// and MariaDB take the cluster-wide lock.
func TestWithMigrationLock_RunsWithoutADatabaseServer(t *testing.T) {
	db := openSQLite(t)

	called := false
	if err := withMigrationLock(db, time.Now().Add(time.Minute), func(got *gorm.DB) error {
		called = true
		if got != db {
			t.Error("the callback did not receive the migration connection")
		}
		return nil
	}); err != nil {
		t.Fatalf("withMigrationLock: %v", err)
	}
	if !called {
		t.Error("the migration did not run")
	}
}

// TestWithMigrationLock_NilDatabase keeps the existing nil check of
// PreMigrateLegacyEvents reachable.
func TestWithMigrationLock_NilDatabase(t *testing.T) {
	want := errors.New("nil db")
	if got := withMigrationLock(nil, time.Now().Add(time.Minute), func(db *gorm.DB) error {
		if db != nil {
			t.Error("expected a nil database")
		}
		return want
	}); !errors.Is(got, want) {
		t.Errorf("err = %v, want %v", got, want)
	}
}

// fakeLocker records the lock lifecycle around the migration.
type fakeLocker struct {
	calls        []string
	acquireErr   error
	configureErr error
}

func (f *fakeLocker) configure(*gorm.DB, time.Time) error {
	if f.configureErr != nil {
		f.calls = append(f.calls, "configure-failed")
		return f.configureErr
	}
	f.calls = append(f.calls, "configure")
	return nil
}

func (f *fakeLocker) reset(*gorm.DB) {
	f.calls = append(f.calls, "reset")
}

func (f *fakeLocker) acquire(*gorm.DB, time.Time) error {
	if f.acquireErr != nil {
		f.calls = append(f.calls, "acquire-failed")
		return f.acquireErr
	}
	f.calls = append(f.calls, "acquire")
	return nil
}

func (f *fakeLocker) release(*gorm.DB) {
	f.calls = append(f.calls, "release")
}

func (f *fakeLocker) sequence(want ...string) bool {
	if len(f.calls) != len(want) {
		return false
	}
	for i := range want {
		if f.calls[i] != want[i] {
			return false
		}
	}
	return true
}

// TestRunWithMigrationLock_HoldsTheLockAroundTheMigration checks the
// lifecycle: configure, acquire, migrate, release, restore.
func TestRunWithMigrationLock_HoldsTheLockAroundTheMigration(t *testing.T) {
	db := openSQLite(t)
	locker := &fakeLocker{}

	err := runWithMigrationLock(db, time.Now().Add(time.Minute), locker, func(*gorm.DB) error {
		locker.calls = append(locker.calls, "migrate")
		return nil
	})
	if err != nil {
		t.Fatalf("runWithMigrationLock: %v", err)
	}
	if !locker.sequence("configure", "acquire", "migrate", "release", "reset") {
		t.Errorf("calls = %v", locker.calls)
	}
}

// TestRunWithMigrationLock_ReleasesWhenTheMigrationFails makes sure a
// failed migration does not leave the cluster lock held.
func TestRunWithMigrationLock_ReleasesWhenTheMigrationFails(t *testing.T) {
	db := openSQLite(t)
	locker := &fakeLocker{}
	want := errors.New("migration failed")

	err := runWithMigrationLock(db, time.Now().Add(time.Minute), locker, func(*gorm.DB) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if !locker.sequence("configure", "acquire", "release", "reset") {
		t.Errorf("calls = %v", locker.calls)
	}
}

// TestRunWithMigrationLock_RestoresWhenTheLockFails makes sure the
// session is restored even when the lock was never acquired.
func TestRunWithMigrationLock_RestoresWhenTheLockFails(t *testing.T) {
	db := openSQLite(t)
	locker := &fakeLocker{acquireErr: errors.New("lock timeout")}

	if err := runWithMigrationLock(db, time.Now().Add(time.Minute), locker, func(*gorm.DB) error {
		t.Error("the migration ran without the lock")
		return nil
	}); err == nil {
		t.Fatal("err = nil, want the lock failure")
	}
	if !locker.sequence("configure", "acquire-failed", "reset") {
		t.Errorf("calls = %v", locker.calls)
	}
}

// TestRunWithMigrationLock_RestoresWhenConfigurationFails keeps the
// session restore unconditional.
func TestRunWithMigrationLock_RestoresWhenConfigurationFails(t *testing.T) {
	db := openSQLite(t)
	locker := &fakeLocker{configureErr: errors.New("set session failed")}

	if err := runWithMigrationLock(db, time.Now().Add(time.Minute), locker, func(*gorm.DB) error {
		t.Error("the migration ran without configuring the session")
		return nil
	}); err == nil {
		t.Fatal("err = nil, want the configuration failure")
	}
	if !locker.sequence("configure-failed", "reset") {
		t.Errorf("calls = %v", locker.calls)
	}
}
