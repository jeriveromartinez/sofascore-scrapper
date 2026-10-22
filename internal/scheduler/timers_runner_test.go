package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/push"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

// unitPusher is a Pusher that blocks for a fixed delay, so a tick
// can be made to last longer than the lease TTL.
type unitPusher struct {
	delay time.Duration
	calls atomic.Int64
}

func (p *unitPusher) PublishPush(_ context.Context, _ uint64, _ *pb.WsPush) error {
	p.calls.Add(1)
	time.Sleep(p.delay)
	return nil
}

type unitDomainRepo struct{ owned map[uint][]domains.Domain }

func (r *unitDomainRepo) ListByUser(_ context.Context, userID uint) ([]domains.Domain, error) {
	return r.owned[userID], nil
}

type unitUserRepo struct{ users map[uint]*users.User }

func (r *unitUserRepo) GetByID(_ context.Context, id uint) (*users.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}

// syncBuffer collects log output written from background goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// timersUnitFixture wires a SQLite-backed push service, a user with
// one domain and one device, so a tick has a due timer to dispatch.
type timersUnitFixture struct {
	db     *gorm.DB
	svc    *push.Service
	pusher *unitPusher
	uid    uint
	domain uint
}

func newTimersUnitFixture(t *testing.T, pusher *unitPusher) *timersUnitFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:timers_unit_%s_%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
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

	u := &users.User{
		Email:                fmt.Sprintf("timers-unit-%d@x.com", time.Now().UnixNano()),
		Password:             "x",
		Role:                 users.RoleUser,
		NotificationsEnabled: true,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("timers-unit-%d.iptv.example", u.ID), UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	uid, did := u.ID, d.ID
	dev := devices.Device{
		UserID:   &uid,
		DomainID: &did,
		Token:    fmt.Sprintf("timers-unit-token-%d", u.ID),
		Platform: "android",
		Name:     "timers-unit",
		Version:  "1.0",
		Timezone: "UTC",
	}
	if err := db.Create(&dev).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}

	repo := push.NewRepository(db)
	dom := &unitDomainRepo{owned: map[uint][]domains.Domain{
		u.ID: {{Model: gorm.Model{ID: d.ID}, UserID: u.ID, Domain: d.Domain}},
	}}
	usr := &unitUserRepo{users: map[uint]*users.User{
		u.ID: {Model: gorm.Model{ID: u.ID}, Email: u.Email, NotificationsEnabled: true},
	}}
	svc := push.NewService(repo, pusher, dom, usr, nil)

	return &timersUnitFixture{
		db:     db,
		svc:    svc,
		pusher: pusher,
		uid:    u.ID,
		domain: d.ID,
	}
}

// seedDueSchedule creates a recurring schedule with one timer per
// device and backdates the timers so the next tick picks them up.
func (f *timersUnitFixture) seedDueSchedule(t *testing.T) []push.ScheduledPushTimer {
	t.Helper()
	_, timers, err := f.svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domain}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "tick", Body: "tick", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *", push.TimezoneModeShared, "UTC")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if len(timers) == 0 {
		t.Fatal("CreateSchedule returned no timers")
	}
	ids := make([]uint, 0, len(timers))
	for _, tm := range timers {
		ids = append(ids, tm.ID)
	}
	if err := f.db.Model(&push.ScheduledPushTimer{}).
		Where("id IN ?", ids).
		Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate timers: %v", err)
	}
	return timers
}

// TestStartTimersRunnerWithoutLockerDisablesRunner asserts that a
// push service without a Redis locker disables the runner instead of
// panicking in the background goroutine: the tick dereferences the
// locker, so a nil locker used to take the whole process down.
func TestStartTimersRunnerWithoutLockerDisablesRunner(t *testing.T) {
	pusher := &unitPusher{}
	f := newTimersUnitFixture(t, pusher)
	f.seedDueSchedule(t)

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTimersRunner(ctx, f.svc, 5*time.Millisecond, 10, nil, logger, &wg)

	// The runner goroutine must not have been started: with a live
	// context wg.Wait only returns when no runner is running.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timers runner was started without a Redis locker")
	}

	if got := pusher.calls.Load(); got != 0 {
		t.Errorf("timers runner dispatched %d pushes without a Redis locker, want 0", got)
	}
	if !strings.Contains(logs.String(), "locker") {
		t.Errorf("expected a warning about the missing locker, got: %s", logs.String())
	}
}

// TestTimersRunnerRenewsLeaseWhileTickRuns asserts that the lease is
// renewed while a tick dispatches. A tick that outruns the lease TTL
// would otherwise let a second instance acquire the same key and
// dispatch the same timers concurrently.
func TestTimersRunnerRenewsLeaseWhileTickRuns(t *testing.T) {
	pusher := &unitPusher{delay: 300 * time.Millisecond}
	f := newTimersUnitFixture(t, pusher)
	f.seedDueSchedule(t)

	lease := &fakeLease{}
	runner := &timersRunner{
		svc:        f.svc,
		locker:     &fakeLocker{lease: lease, acquired: true},
		logger:     slog.New(slog.NewTextHandler(&syncBuffer{}, nil)),
		batchLimit: 100,
		lockTTL:    90 * time.Millisecond,
	}

	runner.RunOnce(context.Background())

	if lease.renews.Load() == 0 {
		t.Fatalf("the lease was never renewed during the tick (renew interval %s, tick %s)",
			90*time.Millisecond/3, pusher.delay)
	}
	if got := runner.firedCount(); got != 1 {
		t.Errorf("firedCount = %d, want 1", got)
	}
}

// TestTimersRunnerReapsStaleClaims asserts the runner releases the
// timers whose claim was left open by a worker that died mid
// dispatch: the push is lost forever otherwise, because the pending
// query skips every row with dispatched_at set.
func TestTimersRunnerReapsStaleClaims(t *testing.T) {
	pusher := &unitPusher{}
	f := newTimersUnitFixture(t, pusher)
	timers := f.seedDueSchedule(t)

	// Simulate the crash: the claim is stamped but the dispatch
	// never happened.
	stale := time.Now().Add(-time.Hour)
	if err := f.db.Model(&push.ScheduledPushTimer{}).
		Where("id = ?", timers[0].ID).
		Updates(map[string]any{"dispatched_at": stale, "claimed_at": stale}).Error; err != nil {
		t.Fatalf("stamp stale claim: %v", err)
	}

	runner := &timersRunner{
		svc:              f.svc,
		locker:           &fakeLocker{lease: &fakeLease{}, acquired: true},
		logger:           slog.New(slog.NewTextHandler(&syncBuffer{}, nil)),
		batchLimit:       100,
		staleClaimWindow: time.Minute,
	}
	runner.RunOnce(context.Background())

	if got := pusher.calls.Load(); got != 1 {
		t.Errorf("published %d pushes, want 1 (the stale claim should be retried)", got)
	}

	var row push.ScheduledPushTimer
	if err := f.db.First(&row, timers[0].ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if row.DispatchedAt == nil {
		t.Error("dispatched_at not stamped after the retry")
	}
	if row.ClaimedAt != nil {
		t.Error("claimed_at not cleared after a completed dispatch")
	}
}

// TestTimersRunnerDoesNotRefireCompletedDispatches is the guard
// against a duplicate-push storm: a dispatch that completed must
// never be re-armed by the reaper, no matter how old the cutoff is.
func TestTimersRunnerDoesNotRefireCompletedDispatches(t *testing.T) {
	pusher := &unitPusher{}
	f := newTimersUnitFixture(t, pusher)
	timers := f.seedDueSchedule(t)

	runner := &timersRunner{
		svc:              f.svc,
		locker:           &fakeLocker{lease: &fakeLease{}, acquired: true},
		logger:           slog.New(slog.NewTextHandler(&syncBuffer{}, nil)),
		batchLimit:       100,
		staleClaimWindow: time.Minute,
	}
	runner.RunOnce(context.Background())
	if got := pusher.calls.Load(); got != 1 {
		t.Fatalf("published %d pushes, want 1", got)
	}

	// A completed dispatch stays completed however old it gets: the
	// cutoff is far in the past, so only the closed claim keeps the
	// reaper from re-firing it.
	if err := f.db.Model(&push.ScheduledPushTimer{}).
		Where("id = ?", timers[0].ID).
		Update("dispatched_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("backdate dispatched_at: %v", err)
	}
	runner.staleClaimWindow = time.Minute
	runner.RunOnce(context.Background())

	if got := pusher.calls.Load(); got != 1 {
		t.Errorf("published %d pushes, want 1 (a completed dispatch must not repeat)", got)
	}
	var row push.ScheduledPushTimer
	if err := f.db.First(&row, timers[0].ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if row.DispatchedAt == nil {
		t.Error("a completed dispatch was re-armed")
	}
}
