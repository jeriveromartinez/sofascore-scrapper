package playback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"gorm.io/gorm"
)

// playbackDBCounter hands out the name of the in-memory database each call
// gets. A shared-cache database survives until its last connection closes,
// so the name has to be unique per call.
var playbackDBCounter atomic.Int64

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// A bare ":memory:" DSN gives every pooled connection its own private
	// database, so concurrent goroutines (used by the lock tests) hit a
	// connection without the migrated tables ("no such table: devices").
	// A named shared-cache in-memory DSN keeps one database shared across the
	// pool, while a name unique per call keeps the calls isolated from each
	// other and from the databases of earlier runs.
	dsn := fmt.Sprintf("file:playback-%d?mode=memory&cache=shared", playbackDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// The pool is closed so the named database does not outlive the test.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&PlaybackLog{}, &devices.Device{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// TestSetupTestDBUsesAFreshDatabasePerCall pins the hermeticity of the
// in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one wrote to.
func TestSetupTestDBUsesAFreshDatabasePerCall(t *testing.T) {
	first := setupTestDB(t)
	if err := first.Create(&devices.Device{Token: "first"}).Error; err != nil {
		t.Fatalf("failed to seed device: %v", err)
	}

	second := setupTestDB(t)
	var count int64
	if err := second.Model(&devices.Device{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count devices: %v", err)
	}
	if count != 0 {
		t.Fatalf("the second database holds %d devices of the first one", count)
	}
}

func TestStartConcurrentOnlyOneOpenLog(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dev := devices.Device{Token: "concurrent"}
	db.Create(&dev)

	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Start(context.Background(), dev.ID, "ch", 1000)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	errCount := 0
	for range errs {
		errCount++
	}
	if errCount > 1 {
		t.Errorf("expected at most 1 error, got %d", errCount)
	}

	var openCount int64
	db.Model(&PlaybackLog{}).Where("device_id = ? AND ended_at = 0", dev.ID).Count(&openCount)
	if openCount != 1 {
		t.Errorf("expected exactly 1 open log, got %d", openCount)
	}
}

func TestStartDeviceLockBlocksConcurrent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dev := devices.Device{Token: "locktest"}
	db.Create(&dev)

	tx1 := db.Begin()
	if err := tx1.Exec("UPDATE devices SET last_seen = 1 WHERE id = ?", dev.ID).Error; err != nil {
		tx1.Rollback()
		t.Fatalf("failed to lock: %v", err)
	}

	type result struct {
		log *PlaybackLog
		err error
	}
	ch := make(chan result, 1)
	go func() {
		log, err := repo.Start(context.Background(), dev.ID, "ch", 1000)
		ch <- result{log, err}
	}()

	tx1.Rollback()

	r := <-ch
	if r.err != nil {
		t.Errorf("expected success after lock released, got: %v", r.err)
	}
	if r.log == nil {
		t.Error("expected non-nil log")
	}
}

func TestStartOnlyClosesEndedAtZero(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dev := devices.Device{Token: "close-test"}
	db.Create(&dev)

	closed := PlaybackLog{DeviceID: dev.ID, Content: "ch1", StartedAt: 100, EndedAt: 200}
	db.Create(&closed)

	open := PlaybackLog{DeviceID: dev.ID, Content: "ch2", StartedAt: 300, EndedAt: 0}
	db.Create(&open)

	_, err := repo.Start(context.Background(), dev.ID, "ch3", 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var c PlaybackLog
	db.First(&c, closed.ID)
	if c.EndedAt != 200 {
		t.Errorf("closed playback ended_at changed from 200 to %d", c.EndedAt)
	}

	var o PlaybackLog
	db.First(&o, open.ID)
	if o.EndedAt != 500 {
		t.Errorf("open playback ended_at expected 500, got %d", o.EndedAt)
	}
}

func TestStartReturnsErrorForMissingDevice(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, err := repo.Start(context.Background(), 9999, "ch", 1000)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestStartInsertFailureRollsBackEverything(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dev := devices.Device{Token: "rollback"}
	db.Create(&dev)

	open := PlaybackLog{DeviceID: dev.ID, Content: "ch", StartedAt: 100, EndedAt: 0}
	db.Create(&open)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Start(ctx, dev.ID, "content", 500)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	var o PlaybackLog
	db.First(&o, open.ID)
	if o.EndedAt != 0 {
		t.Errorf("open playback was closed despite cancelled context, ended_at=%d", o.EndedAt)
	}

	var updatedDev devices.Device
	db.First(&updatedDev, dev.ID)
	if updatedDev.LastSeen != 0 {
		t.Errorf("device last_seen was updated despite cancelled context, got %d", updatedDev.LastSeen)
	}
}

// TestTotalCountReturnsErrorOnCancelledContext verifies that
// TotalCount propagates the request context. The previous
// implementation discarded the gorm error via `_ =`, so a
// cancelled context would silently return a (stale) zero.
func TestTotalCountReturnsErrorOnCancelledContext(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count, err := repo.TotalCount(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 on error", count)
	}
}

// TestTotalCountReturnsActualCount is the happy-path counterpart:
// with an open context the function must return the persisted
// count and no error.
func TestTotalCountReturnsActualCount(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	dev := devices.Device{Token: "totals"}
	db.Create(&dev)
	for i := 0; i < 5; i++ {
		db.Create(&PlaybackLog{DeviceID: dev.ID, Content: "ch", StartedAt: int64(i * 100)})
	}

	count, err := repo.TotalCount(context.Background())
	if err != nil {
		t.Fatalf("TotalCount: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}
