package playback

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&PlaybackLog{}, &devices.Device{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
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
