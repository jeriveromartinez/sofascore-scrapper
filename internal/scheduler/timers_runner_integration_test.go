//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"log/slog"
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
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// counters used to disambiguate test SQLite databases (each
// test gets its own shared memory DB).
var timersRunnerTestCounter int64

// minimal pusher for integration tests; records dispatched
// schedule IDs without trying to talk to FCM.
type recordingPusher struct{ count atomic.Int64 }

func (r *recordingPusher) PublishPush(_ context.Context, _ uint64, _ *pb.WsPush) error {
	r.count.Add(1)
	return nil
}

type fixtureDomainRepo struct{ owned map[uint][]domains.Domain }

func (f *fixtureDomainRepo) ListByUser(_ context.Context, userID uint) ([]domains.Domain, error) {
	return f.owned[userID], nil
}

type fixtureUserRepo struct{ users map[uint]*users.User }

func (f *fixtureUserRepo) GetByID(id uint) (*users.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}

// timersRunnerFixture wires together the SQLite-backed repo,
// miniredis-backed timerstore, recordingPusher and timersRunner
// needed to exercise the worker without booting the full app.
type timersRunnerFixture struct {
	db      *gorm.DB
	rdb     *goredis.Client
	mr      *miniredis.Miniredis
	store   *redisplatform.RedisTimerStore
	repo    *push.Repository
	svc     *push.Service
	pusher  *recordingPusher
	runner  *timersRunner
	devices []devices.Device
	uid     uint
	domain  uint
}

func newTimersRunnerFixture(t *testing.T) *timersRunnerFixture {
	t.Helper()
	id := atomic.AddInt64(&timersRunnerTestCounter, 1)
	dsn := fmt.Sprintf("file:test_timrunner_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&users.User{}, &domains.Domain{}, &devices.Device{},
		&push.PushMessage{}, &push.PushMessageTarget{},
		&push.ScheduledPush{}, &push.ScheduledPushTarget{}, &push.ScheduledPushTimer{},
		&push.DeliveryAttempt{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	u := &users.User{
		Email:                fmt.Sprintf("tr-%d@x.com", time.Now().UnixNano()),
		Password:             "x",
		Role:                 users.RoleUser,
		NotificationsEnabled: true,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("tr-%d.iptv.example", u.ID), UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	repo := push.NewRepository(db)
	store := redisplatform.NewTimerStore(rdb)
	pusher := &recordingPusher{}
	dom := &fixtureDomainRepo{owned: map[uint][]domains.Domain{
		u.ID: {{Model: gorm.Model{ID: d.ID}, UserID: u.ID, Domain: d.Domain}},
	}}
	usr := &fixtureUserRepo{users: map[uint]*users.User{
		u.ID: {Model: gorm.Model{ID: u.ID}, Email: u.Email, NotificationsEnabled: true},
	}}
	svc := push.NewService(repo, pusher, dom, usr, nil)

	runner := &timersRunner{
		svc:        svc,
		store:      store,
		locker:     redisplatform.NewLocker(rdb),
		logger:     slog.New(slog.NewTextHandler(timerRunnerLog{t}, nil)),
		batchLimit: 100,
	}
	return &timersRunnerFixture{
		db: db, rdb: rdb, mr: mr, store: store, repo: repo, svc: svc,
		pusher: pusher, runner: runner,
		uid: u.ID, domain: d.ID,
	}
}

type timerRunnerLog struct{ t *testing.T }

func (w timerRunnerLog) Write(b []byte) (int, error) { w.t.Log(string(b)); return len(b), nil }

func (f *timersRunnerFixture) addDevice(token, tz string) devices.Device {
	uidP := f.uid
	didP := f.domain
	d := devices.Device{
		UserID:   &uidP,
		DomainID: &didP,
		Token:    token,
		Platform: "android",
		Name:     token,
		Version:  "1.0",
		Timezone: tz,
	}
	if err := f.db.Create(&d).Error; err != nil {
		f.runner.logger.Error("seed device", "err", err)
	}
	f.devices = append(f.devices, d)
	return d
}

func TestTimersRunner_DispatchesDueTimersFromRedis(t *testing.T) {
	f := newTimersRunnerFixture(t)
	f.addDevice("d-1", "UTC")
	f.addDevice("d-2", "UTC")

	sched, timers, err := f.svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domain}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "news", Body: "now", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *", push.TimezoneModeManager, "UTC")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if len(timers) != 2 {
		t.Fatalf("want 2 timers, got %d", len(timers))
	}

	// Push the timers into Redis and backdate them in DB so the
	// runner treats them as due.
	entries := make([]redisplatform.TimerEntry, 0, len(timers))
	for _, tm := range timers {
		entries = append(entries, redisplatform.TimerEntry{DeviceID: tm.DeviceID, FireAt: tm.FireAt})
	}
	if err := f.store.Enqueue(context.Background(), sched.ID, entries); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := f.db.Model(&push.ScheduledPushTimer{}).Where("scheduled_push_id = ?", sched.ID).Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	f.runner.RunOnce(context.Background())

	// Original 2 timers must be stamped dispatched in DB.
	var originalIDs []uint
	for _, tm := range timers {
		originalIDs = append(originalIDs, tm.ID)
	}
	var stamped int64
	f.db.Model(&push.ScheduledPushTimer{}).Where("id IN ?", originalIDs).Not("dispatched_at", nil).Count(&stamped)
	if stamped != int64(len(originalIDs)) {
		t.Errorf("want all %d original timers dispatched, got %d", len(originalIDs), stamped)
	}

	// For recurring schedules the runner should have created
	// one next-fire row per device and re-enqueued them in Redis.
	var next int64
	f.db.Model(&push.ScheduledPushTimer{}).Where("scheduled_push_id = ? AND dispatched_at IS NULL", sched.ID).Count(&next)
	if next != int64(len(originalIDs)) {
		t.Errorf("want %d next-fire rows, got %d", len(originalIDs), next)
	}

	// RunOnce has already moved them into Redis (reEnqueueNext).
	got, err := f.store.PopDue(context.Background(), sched.ID, time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("scan redis: %v", err)
	}
	if len(got) != len(originalIDs) {
		t.Errorf("Redis ZSET has %d next-fire entries, want %d", len(got), len(originalIDs))
	}
}

func TestTimersRunner_MarkDispatchedIsIdempotent(t *testing.T) {
	// Two workers race to dispatch the same timer. Only one
	// row update succeeds (MarkTimerDispatched uses a
	// `dispatched_at IS NULL` guard); the loser sees an error
	// or no-op. Either way, the row ends up with exactly one
	// dispatch stamp and the recordingPusher fires twice at
	// most once per device.
	f := newTimersRunnerFixture(t)
	f.addDevice("d-1", "UTC")

	_, timers, err := f.svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domain}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "x", Body: "x", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *", push.TimezoneModeManager, "UTC")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	timer := timers[0]
	if err := f.db.Model(&push.ScheduledPushTimer{}).Where("id = ?", timer.ID).Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Simulate the race: dispatch twice and verify MarkTimerDispatched
	// only stamps the row once.
	if _, err := f.repo.MarkTimerDispatched(context.Background(), timer.ID, time.Now()); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if _, err := f.repo.MarkTimerDispatched(context.Background(), timer.ID, time.Now()); err != nil {
		t.Fatalf("second mark: %v", err)
	}
	var row push.ScheduledPushTimer
	if err := f.db.First(&row, timer.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if row.DispatchedAt == nil {
		t.Fatal("DispatchedAt is nil")
	}
}