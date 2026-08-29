//go:build integration

package push

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

var repoTestCounter int64

// newRepoDB returns a fresh GORM DB for each test, with the minimal
// schema needed by the push repository. Uses shared-cache in-memory
// SQLite; the underlying *sql.DB is closed via t.Cleanup.
func newRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&repoTestCounter, 1)
	dsn := fmt.Sprintf("file:test_pushrepo_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&users.User{}, &domains.Domain{}, &devices.Device{},
		&PushMessage{}, &PushMessageTarget{},
		&ScheduledPush{}, &ScheduledPushTarget{},
		&DeliveryAttempt{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		_ = filepath.Join("ignored") // keep the import
	})
	return db
}

// seedUserDomainDevice inserts a user, a domain, and N devices —
// the minimum fixture for any push repository test. Returns the IDs.
func seedUserDomainDevice(t *testing.T, db *gorm.DB, n int) (uint, uint, []uint) {
	t.Helper()
	u := &users.User{Email: fmt.Sprintf("u-%d@x.com", time.Now().UnixNano()), Password: "x", Role: users.RoleUser}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("d-%d.iptv.example", u.ID), UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	deviceIDs := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		didP := d.ID
		uidP := u.ID
		dev := &devices.Device{
			UserID:   &uidP,
			DomainID: &didP,
			Token:    fmt.Sprintf("tok-%d-%d", u.ID, i),
			Platform: "android",
			Name:     fmt.Sprintf("box-%d", i),
			Version:  "1.0",
		}
		if err := db.Create(dev).Error; err != nil {
			t.Fatalf("seed device %d: %v", i, err)
		}
		deviceIDs = append(deviceIDs, dev.ID)
	}
	return u.ID, d.ID, deviceIDs
}

// TestRepository_InsertPushMessageWithTargets covers the most common
// write path: create a push and link it to two domains in a single
// transaction.
func TestRepository_InsertPushMessageWithTargets(t *testing.T) {
	db := newRepoDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 0)
	d2 := &domains.Domain{Domain: "second.iptv.example", UserID: uid}
	if err := db.Create(d2).Error; err != nil {
		t.Fatalf("seed d2: %v", err)
	}

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	msg := &PushMessage{
		UserID:   uid,
		Category: CategoryAdminMessage,
		Title:    "Hi",
		Body:     "World",
		Priority: PriorityNormal,
		Source:   SourceImmediate,
	}
	if err := repo.InsertPushMessageWithTargets(ctx, msg, []uint{d1, d2.ID}, &now); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if msg.ID == 0 {
		t.Fatal("ID not assigned after insert")
	}

	// Re-read and assert both targets persisted.
	var got PushMessage
	if err := db.Preload("Domains").First(&got, msg.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(got.Domains) != 2 {
		t.Errorf("domains persisted = %d, want 2", len(got.Domains))
	}
}

// TestRepository_ListAudienceDevicesForDomains covers the audience
// filter that drives the push delivery: devices whose domain_id
// matches the push's target domains AND whose domain_id is non-null.
// Devices without a domain are excluded (the contract: they cannot
// receive pushes).
func TestRepository_ListAudienceDevicesForDomains(t *testing.T) {
	db := newRepoDB(t)
	uid, d1, deviceIDs := seedUserDomainDevice(t, db, 3)

	// Add a device with no domain (must be excluded).
	uidP := uid
	orphan := &devices.Device{UserID: &uidP, Token: "orphan", Platform: "android", Name: "no-domain", Version: "1.0"}
	if err := db.Create(orphan).Error; err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	// Add a device under a different domain (also excluded).
	d3 := &domains.Domain{Domain: "other.iptv.example", UserID: uid}
	if err := db.Create(d3).Error; err != nil {
		t.Fatalf("seed d3: %v", err)
	}
	d3ID := d3.ID
	uidP2 := uid
	stranger := &devices.Device{UserID: &uidP2, DomainID: &d3ID, Token: "stranger", Platform: "android", Name: "x", Version: "1.0"}
	if err := db.Create(stranger).Error; err != nil {
		t.Fatalf("seed stranger: %v", err)
	}

	repo := NewRepository(db)
	got, err := repo.ListAudienceDevicesForDomains(context.Background(), []uint{d1})
	if err != nil {
		t.Fatalf("list audience: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("audience size = %d, want 3", len(got))
	}
	for _, d := range got {
		if d.ID == orphan.ID {
			t.Error("orphan (no domain) was included")
		}
		if d.ID == stranger.ID {
			t.Error("stranger (other domain) was included")
		}
	}
	// All three returned devices must be the ones we seeded.
	want := map[uint]bool{}
	for _, id := range deviceIDs {
		want[id] = true
	}
	for _, d := range got {
		if !want[d.ID] {
			t.Errorf("unexpected device %d in audience", d.ID)
		}
	}
}

// TestRepository_MarkDeliverySent_Delivered_Failed walks the
// delivery_attempts lifecycle. After Insert, the row is SENT; after
// MarkDelivered, the state flips and latency_ms is set; the
// separate MarkFailed path produces a different final state with
// a failure_reason.
func TestRepository_MarkDeliverySent_Delivered_Failed(t *testing.T) {
	db := newRepoDB(t)
	uid, d1, devs := seedUserDomainDevice(t, db, 1)
	_ = uid

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()
	msg := &PushMessage{
		UserID: uid, Category: CategoryEventAlert, Title: "t", Body: "b",
		Priority: PriorityNormal, Source: SourceImmediate,
	}
	if err := repo.InsertPushMessageWithTargets(ctx, msg, []uint{d1}, &now); err != nil {
		t.Fatalf("insert: %v", err)
	}
	attempts := []DeliveryAttempt{{
		PushMessageID: msg.ID, DeviceID: devs[0],
		MessageID:     "test-msg-1",
		State:         StateSent, SentAt: &now,
	}}
	if err := repo.InsertDeliveryAttempts(ctx, attempts); err != nil {
		t.Fatalf("insert attempts: %v", err)
	}
	ackedAt := now.Add(50 * time.Millisecond)
	if err := repo.MarkDeliveryDelivered(ctx, msg.ID, devs[0], ackedAt); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	var got DeliveryAttempt
	if err := db.First(&got, "push_message_id = ? AND device_id = ?", msg.ID, devs[0]).Error; err != nil {
		t.Fatalf("re-read attempt: %v", err)
	}
	if got.State != StateDelivered {
		t.Errorf("state = %q, want delivered", got.State)
	}
	if got.AckedAt == nil {
		t.Error("acked_at not set")
	}
	if got.LatencyMS == nil || *got.LatencyMS < 40 || *got.LatencyMS > 200 {
		t.Errorf("latency_ms = %v, want ~50", got.LatencyMS)
	}

	// Now exercise the failure path on a fresh attempt.
	msg2 := &PushMessage{
		UserID: uid, Category: CategoryEventAlert, Title: "t2", Body: "b2",
		Priority: PriorityNormal, Source: SourceImmediate,
	}
	if err := repo.InsertPushMessageWithTargets(ctx, msg2, []uint{d1}, &now); err != nil {
		t.Fatalf("insert msg2: %v", err)
	}
	if err := repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{{
		PushMessageID: msg2.ID, DeviceID: devs[0],
		MessageID:     "test-msg-2",
		State:         StateSent, SentAt: &now,
	}}); err != nil {
		t.Fatalf("insert attempts2: %v", err)
	}
	if err := repo.MarkDeliveryFailed(ctx, msg2.ID, devs[0], FailureDeviceOffline); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	var got2 DeliveryAttempt
	if err := db.First(&got2, "push_message_id = ? AND device_id = ?", msg2.ID, devs[0]).Error; err != nil {
		t.Fatalf("re-read attempt2: %v", err)
	}
	if got2.State != StateFailed {
		t.Errorf("state = %q, want failed", got2.State)
	}
	if got2.FailureReason != FailureDeviceOffline {
		t.Errorf("failure_reason = %q, want device_offline", got2.FailureReason)
	}
}

// TestRepository_ScheduledPushLifecycle covers the create/list/due/
// fire/reschedule cycle. The recurring path updates next_fire_at
// based on the cron expression; the one_shot path deactivates the
// row and stamps last_fired_at.
func TestRepository_ScheduledPushLifecycle(t *testing.T) {
	db := newRepoDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 0)
	_ = d1

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Create a one_shot in the past relative to "now+1s" so it
	// shows up in the due query immediately.
	runAt := now.Add(-time.Minute)
	one := &ScheduledPush{
		UserID:       uid,
		ScheduleType: ScheduleTypeOneShot,
		RunAt:        &runAt,
		NextFireAt:   runAt,
		IsActive:     true,
		Category:     CategoryAdminMessage,
		Title:        "one",
		Body:         "shot",
		Priority:     PriorityNormal,
	}
	if err := repo.InsertScheduledPushWithTargets(ctx, one, []uint{d1}); err != nil {
		t.Fatalf("insert one: %v", err)
	}

	// Create a recurring that fires hourly.
	rec := &ScheduledPush{
		UserID:       uid,
		ScheduleType: ScheduleTypeRecurring,
		CronExpr:     "0 * * * *",
		NextFireAt:   now.Add(-time.Hour),
		IsActive:     true,
		Category:     CategoryAdminMessage,
		Title:        "rec",
		Body:         "urring",
		Priority:     PriorityNormal,
	}
	if err := repo.InsertScheduledPushWithTargets(ctx, rec, []uint{d1}); err != nil {
		t.Fatalf("insert rec: %v", err)
	}

	due, err := repo.ListDueScheduledPushes(ctx, now, 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("due count = %d, want 2", len(due))
	}

	// Mark one as fired and deactivate.
	if err := repo.MarkScheduledPushFired(ctx, one.ID, now); err != nil {
		t.Fatalf("mark fired: %v", err)
	}
	var got ScheduledPush
	if err := db.First(&got, one.ID).Error; err != nil {
		t.Fatalf("re-read one: %v", err)
	}
	if got.IsActive {
		t.Error("one_shot should be deactivated after firing")
	}
	if got.LastFiredAt == nil {
		t.Error("last_fired_at not set")
	}

	// Reschedule the recurring.
	next := now.Add(time.Hour)
	if err := repo.RescheduleRecurring(ctx, rec.ID, next, now); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	var gotRec ScheduledPush
	if err := db.First(&gotRec, rec.ID).Error; err != nil {
		t.Fatalf("re-read rec: %v", err)
	}
	if !gotRec.NextFireAt.Equal(next) {
		t.Errorf("next_fire_at = %v, want %v", gotRec.NextFireAt, next)
	}
	if !gotRec.IsActive {
		t.Error("recurring should remain active")
	}
}

// TestRepository_DeactivateAllForUser covers the cascade that
// happens when a user toggles notifications_enabled off: every
// active schedule for the user is deactivated in a single UPDATE
// so the next push attempt does not try to fire.
func TestRepository_DeactivateAllForUser(t *testing.T) {
	db := newRepoDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 0)
	_ = d1

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 3; i++ {
		s := &ScheduledPush{
			UserID: uid, ScheduleType: ScheduleTypeRecurring,
			CronExpr: "0 * * * *", NextFireAt: now.Add(time.Hour),
			IsActive: true, Category: CategoryAdminMessage,
			Title: fmt.Sprintf("s%d", i), Body: "b", Priority: PriorityNormal,
		}
		if err := repo.InsertScheduledPushWithTargets(ctx, s, []uint{d1}); err != nil {
			t.Fatalf("seed s%d: %v", i, err)
		}
	}
	if err := repo.DeactivateAllForUser(ctx, uid); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	var n int64
	if err := db.Model(&ScheduledPush{}).Where("user_id = ? AND is_active = ?", uid, true).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("active after deactivate = %d, want 0", n)
	}
}
