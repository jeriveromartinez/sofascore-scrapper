//go:build integration

package push

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// newServiceDB returns a SQLite-shared GORM DB with every push
// table. It also closes the underlying *sql.DB via t.Cleanup so the
// shared cache is released before the next test.
func newServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&svcTestCounter, 1)
	dsn := fmt.Sprintf("file:test_svc_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
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
	})
	return db
}

var svcTestCounter int64

// callCount is defined here (rather than in service_test.go) so it is
// scoped behind //go:build integration and invisible to the default lint
// scope. The unit-test linter correctly flags the unused-symbol rule for
// the default scope; the integration tests carry their own coverage.
func (f *fakePusher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestService_CreateImmediate_DispatchesAndPersists is the
// end-to-end happy path:
//  1. user has notifications_enabled = true
//  2. domain_id is owned by the user
//  3. audience is 3 devices
//  4. pusher returns nil for all 3
//  5. 3 push_calls are recorded on the pusher
//  6. 3 delivery_attempts rows are inserted with state = sent
func TestService_CreateImmediate_DispatchesAndPersists(t *testing.T) {
	db := newServiceDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 3)

	repo := NewRepository(db)
	pusher := newFakePusher()
	domainRepo := &fakeDomainRepo{owned: map[uint][]domains.Domain{
		uid: {domainOwnedBy(uid, d1)},
	}}
	userRepo := &fakeUserRepo{users: map[uint]*users.User{uid: userWith(true)}}
	// Mark the user notifications_enabled=true in the DB too.
	if err := db.Model(&users.User{}).Where("id = ?", uid).Update("notifications_enabled", true).Error; err != nil {
		t.Fatalf("enable user: %v", err)
	}
	s := NewService(repo, pusher, domainRepo, userRepo, nil)

	msg, audience, err := s.CreateImmediate(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "Hello", Body: "World",
		Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	})
	if err != nil {
		t.Fatalf("CreateImmediate: %v", err)
	}
	if msg.ID == 0 {
		t.Fatal("msg.ID not assigned")
	}
	if len(audience) != 3 {
		t.Errorf("audience size = %d, want 3", len(audience))
	}
	if pusher.callCount() != 3 {
		t.Errorf("pusher call count = %d, want 3", pusher.callCount())
	}
	// Each pusher call must carry the same push_id (the broadcast
	// is one push with three recipients, not three pushes).
	for i, call := range pusher.calls {
		if call.push.PushId != uint64(msg.ID) {
			t.Errorf("call %d push_id = %d, want %d", i, call.push.PushId, msg.ID)
		}
	}
	// Three delivery_attempts rows persisted.
	var rows []DeliveryAttempt
	if err := db.Find(&rows, "push_message_id = ?", msg.ID).Error; err != nil {
		t.Fatalf("re-read attempts: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("delivery_attempts = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if r.State != StateSent {
			t.Errorf("state = %q, want sent", r.State)
		}
		if r.SentAt == nil {
			t.Error("sent_at not stamped")
		}
	}
}

// TestService_CreateImmediate_RecordsDeviceOffline covers the
// "device not on this backend" path: pusher returns
// ErrDeviceNotConnected, the service records the attempt as
// failed:device_offline.
func TestService_CreateImmediate_RecordsDeviceOffline(t *testing.T) {
	db := newServiceDB(t)
	uid, d1, devs := seedUserDomainDevice(t, db, 2)

	repo := NewRepository(db)
	pusher := newFakePusher()
	pusher.result[uint64(devs[0])] = realtime.ErrDeviceNotConnected
	domainRepo := &fakeDomainRepo{owned: map[uint][]domains.Domain{uid: {domainOwnedBy(uid, d1)}}}
	userRepo := &fakeUserRepo{users: map[uint]*users.User{uid: userWith(true)}}
	if err := db.Model(&users.User{}).Where("id = ?", uid).Update("notifications_enabled", true).Error; err != nil {
		t.Fatalf("enable user: %v", err)
	}
	s := NewService(repo, pusher, domainRepo, userRepo, nil)

	msg, _, err := s.CreateImmediate(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	})
	if err != nil {
		t.Fatalf("CreateImmediate: %v", err)
	}
	if pusher.callCount() != 2 {
		t.Errorf("pusher calls = %d, want 2", pusher.callCount())
	}
	var got DeliveryAttempt
	if err := db.First(&got, "push_message_id = ? AND device_id = ?", msg.ID, devs[0]).Error; err != nil {
		t.Fatalf("re-read offline attempt: %v", err)
	}
	if got.State != StateFailed {
		t.Errorf("state = %q, want failed", got.State)
	}
	if got.FailureReason != FailureDeviceOffline {
		t.Errorf("failure_reason = %q, want device_offline", got.FailureReason)
	}
}

// TestService_OnAck_FlipsRowToDelivered covers the ack path: after
// the client echoes a WsPushAck, the row must be DELIVERED with a
// non-zero latency.
func TestService_OnAck_FlipsRowToDelivered(t *testing.T) {
	db := newServiceDB(t)
	uid, d1, devs := seedUserDomainDevice(t, db, 1)

	repo := NewRepository(db)
	pusher := newFakePusher()
	domainRepo := &fakeDomainRepo{owned: map[uint][]domains.Domain{uid: {domainOwnedBy(uid, d1)}}}
	userRepo := &fakeUserRepo{users: map[uint]*users.User{uid: userWith(true)}}
	if err := db.Model(&users.User{}).Where("id = ?", uid).Update("notifications_enabled", true).Error; err != nil {
		t.Fatalf("enable user: %v", err)
	}
	s := NewService(repo, pusher, domainRepo, userRepo, nil)

	msg, _, err := s.CreateImmediate(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	})
	if err != nil {
		t.Fatalf("CreateImmediate: %v", err)
	}
	// Simulate a client ack 50ms later.
	time.Sleep(50 * time.Millisecond)
	// Look up the message_id from the delivery_attempt row that was
	// inserted by CreateImmediate.
	var attempt DeliveryAttempt
	if err := db.First(&attempt, "push_message_id = ? AND device_id = ?", msg.ID, devs[0]).Error; err != nil {
		t.Fatalf("re-read attempt row: %v", err)
	}
	if err := s.OnAck(context.Background(), attempt.MessageID); err != nil {
		t.Fatalf("OnAck: %v", err)
	}
	var got DeliveryAttempt
	if err := db.First(&got, "push_message_id = ? AND device_id = ?", msg.ID, devs[0]).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got.State != StateDelivered {
		t.Errorf("state = %q, want delivered", got.State)
	}
	if got.AckedAt == nil {
		t.Error("acked_at not set")
	}
	if got.LatencyMS == nil {
		t.Fatal("latency_ms not set")
	}
	if *got.LatencyMS < 40 {
		t.Errorf("latency_ms = %d, want >= 40", *got.LatencyMS)
	}
}

// TestService_CreateSchedule_OneShotAndRecurring exercises both
// schedule types end-to-end. We assert that the persisted row
// matches what the service computed (next_fire_at, cron_expr).
func TestService_CreateSchedule_OneShotAndRecurring(t *testing.T) {
	db := newServiceDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 0)

	repo := NewRepository(db)
	pusher := newFakePusher()
	domainRepo := &fakeDomainRepo{owned: map[uint][]domains.Domain{uid: {domainOwnedBy(uid, d1)}}}
	userRepo := &fakeUserRepo{users: map[uint]*users.User{uid: userWith(true)}}
	if err := db.Model(&users.User{}).Where("id = ?", uid).Update("notifications_enabled", true).Error; err != nil {
		t.Fatalf("enable user: %v", err)
	}
	s := NewService(repo, pusher, domainRepo, userRepo, nil)

	// one_shot
	runAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	one, err := s.CreateSchedule(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT, &runAt, "")
	if err != nil {
		t.Fatalf("CreateSchedule one: %v", err)
	}
	if one.ScheduleType != ScheduleTypeOneShot {
		t.Errorf("schedule_type = %q, want one_shot", one.ScheduleType)
	}
	if one.RunAt == nil || !one.RunAt.Equal(runAt) {
		t.Errorf("run_at = %v, want %v", one.RunAt, runAt)
	}

	// recurring
	rec, err := s.CreateSchedule(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t2", Body: "b2", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *")
	if err != nil {
		t.Fatalf("CreateSchedule rec: %v", err)
	}
	if rec.ScheduleType != ScheduleTypeRecurring {
		t.Errorf("schedule_type = %q, want recurring", rec.ScheduleType)
	}
	if rec.CronExpr != "0 * * * *" {
		t.Errorf("cron_expr = %q", rec.CronExpr)
	}
	// next_fire_at should be the next whole-hour boundary.
	now := time.Now()
	if rec.NextFireAt.Before(now) {
		t.Errorf("next_fire_at = %v, want in the future", rec.NextFireAt)
	}
}

// TestService_CreateSchedule_BadCronIsRejected covers the parser
// path: an unparseable cron expression returns ErrInvalidSchedule
// before the insert.
func TestService_CreateSchedule_BadCronIsRejected(t *testing.T) {
	db := newServiceDB(t)
	uid, d1, _ := seedUserDomainDevice(t, db, 0)

	repo := NewRepository(db)
	pusher := newFakePusher()
	domainRepo := &fakeDomainRepo{owned: map[uint][]domains.Domain{uid: {domainOwnedBy(uid, d1)}}}
	userRepo := &fakeUserRepo{users: map[uint]*users.User{uid: userWith(true)}}
	if err := db.Model(&users.User{}).Where("id = ?", uid).Update("notifications_enabled", true).Error; err != nil {
		t.Fatalf("enable user: %v", err)
	}
	s := NewService(repo, pusher, domainRepo, userRepo, nil)

	_, err := s.CreateSchedule(context.Background(), uid, uid, []uint{d1}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "not a cron")
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Errorf("err = %v, want ErrInvalidSchedule", err)
	}
}
