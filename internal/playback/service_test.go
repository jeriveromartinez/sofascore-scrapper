package playback

import (
	"context"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
)

func TestServiceStartDeviceNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))

	dev := devices.Device{Token: "phantom"}
	dev.ID = 9999

	_, err := svc.Start(context.Background(), dev, "ch1", 1000)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestServiceStartUpdatesLastSeen(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))

	dev := devices.Device{Token: "lastseen-test"}
	db.Create(&dev)

	_, err := svc.Start(context.Background(), dev, "ch1", 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updatedDev devices.Device
	db.First(&updatedDev, dev.ID)
	if updatedDev.LastSeen != 5000 {
		t.Errorf("device last_seen expected 5000, got %d", updatedDev.LastSeen)
	}
}

func TestServiceStartInsertFailureRollsBack(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))

	dev := devices.Device{Token: "rollback-test"}
	db.Create(&dev)

	open := PlaybackLog{DeviceID: dev.ID, Content: "ch", StartedAt: 100, EndedAt: 0}
	db.Create(&open)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Start(ctx, dev, "content", 500)
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

func TestServiceStartCreatesNewLog(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))

	dev := devices.Device{Token: "newlog-test"}
	db.Create(&dev)

	log, err := svc.Start(context.Background(), dev, "ch1", 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if log.DeviceID != dev.ID {
		t.Errorf("expected deviceID %d, got %d", dev.ID, log.DeviceID)
	}
	if log.Content != "ch1" {
		t.Errorf("expected content 'ch1', got %q", log.Content)
	}
	if log.StartedAt != 3000 {
		t.Errorf("expected startedAt 3000, got %d", log.StartedAt)
	}
	if log.EndedAt != 0 {
		t.Errorf("expected endedAt 0, got %d", log.EndedAt)
	}
}
