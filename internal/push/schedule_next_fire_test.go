package push

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var nextFireTestCounter int64

// newNextFireDB returns an in-memory SQLite DB with the schedule
// tables migrated.
func newNextFireDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&nextFireTestCounter, 1)
	dsn := fmt.Sprintf("file:test_next_fire_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ScheduledPush{}, &ScheduledPushTimer{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// TestScheduleNextRecurringFireReportsFailedNextFireAtUpdate covers the
// update of the schedule header after a recurring fire: the error was
// discarded, so a failed update left the schedule showing a stale next
// fire time without any trace.
func TestScheduleNextRecurringFireReportsFailedNextFireAtUpdate(t *testing.T) {
	db := newNextFireDB(t)

	schedule := &ScheduledPush{
		UserID:       1,
		ScheduleType: ScheduleTypeRecurring,
		CronExpr:     "*/5 * * * *",
		NextFireAt:   time.Now().Add(time.Hour),
		IsActive:     true,
		TimezoneMode: TimezoneModeShared,
		Timezone:     "UTC",
		Category:     CategoryScheduled,
		Title:        "t",
		Body:         "b",
	}
	if err := db.Create(schedule).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	// Drop the schedule table so the update of next_fire_at fails
	// while the pending timer row is still written.
	if err := db.Migrator().DropTable(&ScheduledPush{}); err != nil {
		t.Fatalf("drop scheduled_pushes: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := NewService(NewRepository(db), newFakePusher(), &fakeDomainRepo{}, &fakeUserRepo{}, logger)

	err := svc.scheduleNextRecurringFire(context.Background(), schedule, devices.Device{Model: gorm.Model{ID: 1}, Timezone: "UTC"}, time.Now())
	if err != nil {
		t.Fatalf("scheduleNextRecurringFire: %v", err)
	}

	out := logs.String()
	if !strings.Contains(out, "next fire time") {
		t.Fatalf("the failed next_fire_at update was not reported: %s", out)
	}
	if !strings.Contains(out, fmt.Sprintf("schedule_id=%d", schedule.ID)) {
		t.Fatalf("the log does not identify the schedule: %s", out)
	}
}

// TestScheduleNextRecurringFireUpdatesNextFireAt covers the happy
// path the report must not break: the header is bumped to the
// earliest pending timer.
func TestScheduleNextRecurringFireUpdatesNextFireAt(t *testing.T) {
	db := newNextFireDB(t)

	schedule := &ScheduledPush{
		UserID:       1,
		ScheduleType: ScheduleTypeRecurring,
		CronExpr:     "*/5 * * * *",
		NextFireAt:   time.Now().Add(time.Hour),
		IsActive:     true,
		TimezoneMode: TimezoneModeShared,
		Timezone:     "UTC",
		Category:     CategoryScheduled,
		Title:        "t",
		Body:         "b",
	}
	if err := db.Create(schedule).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := NewService(NewRepository(db), newFakePusher(), &fakeDomainRepo{}, &fakeUserRepo{}, logger)

	if err := svc.scheduleNextRecurringFire(context.Background(), schedule, devices.Device{Model: gorm.Model{ID: 1}, Timezone: "UTC"}, time.Now()); err != nil {
		t.Fatalf("scheduleNextRecurringFire: %v", err)
	}

	var stored ScheduledPush
	if err := db.First(&stored, schedule.ID).Error; err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !stored.NextFireAt.After(time.Now()) {
		t.Fatalf("next_fire_at = %v, want a future time", stored.NextFireAt)
	}
	if out := logs.String(); strings.Contains(out, "next fire time") {
		t.Fatalf("a successful update was reported as a failure: %s", out)
	}
}
