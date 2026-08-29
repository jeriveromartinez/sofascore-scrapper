//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/push"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var pushRunTestCounter int64

// pushRunFixture builds the minimum environment for a push runner
// tick: a SQLite DB with the push schema, a miniredis-backed
// Redis client, and a push.Service wired to a fake pusher that
// records every PublishPush call.
type pushRunFixture struct {
	db     *gorm.DB
	redis  *redis.Client
	mr     *miniredis.Miniredis
	runner *pushRunner
	pusher *recordingPusher
	svc    *push.Service
}

type recordingPusher struct {
	calls  []recordingPusherCall
	closed bool
}

type recordingPusherCall struct {
	deviceID uint64
	push     *pbPushSnapshot
}

// pbPushSnapshot is a tiny mirror of pb.WsPush so the test does not
// have to import the gen package. We just keep the push_id so the
// test can assert "this schedule was dispatched".
type pbPushSnapshot struct {
	PushID uint64
}

func newPushRunFixture(t *testing.T) *pushRunFixture {
	t.Helper()
	id := atomic.AddInt64(&pushRunTestCounter, 1)
	dsn := fmt.Sprintf("file:test_pushrun_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(
		&users.User{}, &domains.Domain{}, &devices.Device{},
		&push.PushMessage{}, &push.PushMessageTarget{},
		&push.ScheduledPush{}, &push.ScheduledPushTarget{},
		&push.DeliveryAttempt{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	locker := redisplatform.NewLocker(rdb)

	// Build the service with the recording pusher.
	repo := push.NewRepository(db)
	pusher := &recordingPusher{}
	domainRepo := &stubDomainRepo{}
	userRepo := &stubUserRepo{}
	svc := push.NewService(repo, pusher, domainRepo, userRepo, nil)
	runner := &pushRunner{svc: svc, locker: locker, logger: nil}
	return &pushRunFixture{db: db, redis: rdb, mr: mr, runner: runner, pusher: pusher, svc: svc}
}

func (p *recordingPusher) PublishPush(_ context.Context, deviceID uint64, push *pb.WsPush) error {
	p.calls = append(p.calls, recordingPusherCall{deviceID: deviceID, push: &pbPushSnapshot{PushID: push.PushId}})
	return nil
}

// stubDomainRepo / stubUserRepo are no-op for the runner tests
// because the schedule is already persisted and the dispatch
// path does not call validateDomains / validateFeatureEnabled.
type stubDomainRepo struct{}

func (s *stubDomainRepo) ListByUser(_ context.Context, _ uint) ([]domains.Domain, error) {
	return nil, nil
}

type stubUserRepo struct{}

func (s *stubUserRepo) GetByID(_ uint) (*users.User, error) { return nil, nil }

// seedScheduledPush inserts a due schedule for the runner to pick
// up. Returns the schedule's id and the audience device id.
func (f *pushRunFixture) seedScheduledPush(t *testing.T, scheduleType push.ScheduleType, cronExpr string, runAt time.Time) (uint, uint, uint) {
	t.Helper()
	u := &users.User{Email: fmt.Sprintf("u-%d@x.com", time.Now().UnixNano()), Password: "x", Role: users.RoleUser, NotificationsEnabled: true}
	if err := f.db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("d-%d.iptv", u.ID), UserID: u.ID}
	if err := f.db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	didP := d.ID
	uidP := u.ID
	dev := &devices.Device{UserID: &uidP, DomainID: &didP, Token: fmt.Sprintf("tok-%d", time.Now().UnixNano()), Platform: "android", Name: "Box", Version: "1.0"}
	if err := f.db.Create(dev).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	sched := &push.ScheduledPush{
		UserID:       u.ID,
		ScheduleType: scheduleType,
		CronExpr:     cronExpr,
		NextFireAt:   runAt,
		IsActive:     true,
		Category:     push.CategoryAdminMessage,
		Title:        "sched",
		Body:         "uled",
		Priority:     push.PriorityNormal,
	}
	if err := f.db.Create(sched).Error; err != nil {
		t.Fatalf("seed sched: %v", err)
	}
	if err := f.db.Create(&push.ScheduledPushTarget{ScheduledPushID: sched.ID, DomainID: d.ID}).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}
	return sched.ID, u.ID, dev.ID
}

// TestPushRunner_FiresDueOneShot covers the happy path: a one_shot
// schedule whose next_fire_at is in the past gets dispatched, the
// pusher is called once for the audience device, the row is
// deactivated, and a push_messages row + delivery_attempts row
// are persisted.
func TestPushRunner_FiresDueOneShot(t *testing.T) {
	f := newPushRunFixture(t)
	schedID, _, devID := f.seedScheduledPush(t, push.ScheduleTypeOneShot, "", time.Now().Add(-time.Minute))

	f.runner.tick(context.Background())

	// Pusher was called once.
	if got := len(f.pusher.calls); got != 1 {
		t.Fatalf("pusher calls = %d, want 1", got)
	}
	if f.pusher.calls[0].deviceID != uint64(devID) {
		t.Errorf("pusher call device_id = %d, want %d", f.pusher.calls[0].deviceID, devID)
	}

	// One push_messages row was created.
	var msgs []push.PushMessage
	if err := f.db.Find(&msgs).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("push_messages count = %d, want 1", len(msgs))
	}
	if msgs[0].Source != push.SourceScheduled {
		t.Errorf("source = %q, want scheduled", msgs[0].Source)
	}

	// One delivery_attempts row was created.
	var attempts []push.DeliveryAttempt
	if err := f.db.Find(&attempts).Error; err != nil {
		t.Fatalf("re-read attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("delivery_attempts = %d, want 1", len(attempts))
	}

	// The schedule was deactivated (one_shot).
	var got push.ScheduledPush
	if err := f.db.First(&got, schedID).Error; err != nil {
		t.Fatalf("re-read sched: %v", err)
	}
	if got.IsActive {
		t.Error("one_shot should be deactivated after firing")
	}
	if got.LastFiredAt == nil {
		t.Error("last_fired_at not set")
	}

	// Counters.
	if f.runner.firedCount() != 1 {
		t.Errorf("fired = %d, want 1", f.runner.firedCount())
	}
	if f.runner.failedCount() != 0 {
		t.Errorf("failed = %d, want 0", f.runner.failedCount())
	}
}

// TestPushRunner_FiresDueRecurring covers the recurring path: the
// row stays active, last_fired_at is stamped, and next_fire_at is
// advanced to the next whole hour (cron "0 * * * *").
func TestPushRunner_FiresDueRecurring(t *testing.T) {
	f := newPushRunFixture(t)
	_, _, _ = f.seedScheduledPush(t, push.ScheduleTypeRecurring, "0 * * * *", time.Now().Add(-time.Minute))

	before := time.Now()
	f.runner.tick(context.Background())

	if f.runner.firedCount() != 1 {
		t.Errorf("fired = %d, want 1", f.runner.firedCount())
	}
	var got push.ScheduledPush
	if err := f.db.First(&got).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !got.IsActive {
		t.Error("recurring should stay active after firing")
	}
	if got.LastFiredAt == nil {
		t.Error("last_fired_at not set")
	}
	// next_fire_at must be in the future and within the next
	// hour (cron is "0 * * * *"). The exact boundary depends on
	// the wall clock at the moment of the tick, so we assert a
	// generous window.
	if !got.NextFireAt.After(before) {
		t.Errorf("next_fire_at = %v, want > before %v", got.NextFireAt, before)
	}
	if got.NextFireAt.Sub(before) > 90*time.Minute {
		t.Errorf("next_fire_at - before = %v, want <= 90m (cron is hourly)", got.NextFireAt.Sub(before))
	}
}

// TestPushRunner_NoDueSchedulesIsNoop covers the empty case: no
// rows, no pusher calls, no error, no counter bump.
func TestPushRunner_NoDueSchedulesIsNoop(t *testing.T) {
	f := newPushRunFixture(t)
	f.runner.tick(context.Background())
	if got := len(f.pusher.calls); got != 0 {
		t.Errorf("pusher calls = %d, want 0", got)
	}
	if f.runner.firedCount() != 0 {
		t.Errorf("fired = %d, want 0", f.runner.firedCount())
	}
}

// TestPushRunner_SkipsFutureSchedules covers the off-by-default
// path: a schedule with next_fire_at in the future is not picked
// up. We assert by inserting a one_shot with a future next_fire_at
// and verifying nothing happens.
func TestPushRunner_SkipsFutureSchedules(t *testing.T) {
	f := newPushRunFixture(t)
	f.seedScheduledPush(t, push.ScheduleTypeOneShot, "", time.Now().Add(time.Hour))
	f.runner.tick(context.Background())
	if got := len(f.pusher.calls); got != 0 {
		t.Errorf("pusher calls = %d, want 0", got)
	}
}
