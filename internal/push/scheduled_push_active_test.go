package push

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

var scheduledPushTestCounter int64

// newScheduledPushDB opens a SQLite database with the push tables
// migrated and a user to own the schedules.
func newScheduledPushDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&scheduledPushTestCounter, 1)
	dsn := fmt.Sprintf("file:test_sched_push_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&users.User{}, &domains.Domain{}, &devices.Device{},
		&PushMessage{}, &PushMessageTarget{},
		&ScheduledPush{}, &ScheduledPushTarget{}, &ScheduledPushTimer{},
		&DeliveryAttempt{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	u := &users.User{Email: fmt.Sprintf("sched-push-%d@x.com", time.Now().UnixNano()), Password: "x", Role: users.RoleUser}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return db
}

// TestInsertScheduledPushKeepsInactiveFlag proves that a schedule
// inserted as paused stays paused.
//
// The column used to carry a `default:true` tag. GORM parses that tag
// into a Go-side default and writes it whenever the struct field is the
// zero value, so the insert stored is_active = true and rewrote the
// caller's struct. A caller that asked for a paused schedule got an
// active one.
func TestInsertScheduledPushKeepsInactiveFlag(t *testing.T) {
	db := newScheduledPushDB(t)
	repo := NewRepository(db)

	var user users.User
	if err := db.First(&user).Error; err != nil {
		t.Fatalf("read user: %v", err)
	}

	runAt := time.Now().UTC()
	schedule := &ScheduledPush{
		UserID:       user.ID,
		ScheduleType: ScheduleTypeOneShot,
		RunAt:        &runAt,
		NextFireAt:   runAt,
		IsActive:     false,
		Category:     CategoryAdminMessage,
		Title:        "paused",
		Body:         "paused",
		Priority:     PriorityNormal,
	}
	if err := repo.InsertScheduledPushWithTargets(context.Background(), schedule, nil, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if schedule.IsActive {
		t.Errorf("the insert rewrote the in-memory schedule to active")
	}

	var stored ScheduledPush
	if err := db.First(&stored, schedule.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.IsActive {
		t.Errorf("stored is_active = true, want false")
	}
}
