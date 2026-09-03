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
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// timerFixture holds the wiring a test needs to exercise
//// per-device timezone scheduling without booting the whole app.
type timerFixture struct {
	db        *gorm.DB
	repo      *Repository
	uid       uint
	domainID  uint
	devices   []devices.Device
}

func newTimerFixture(t *testing.T) *timerFixture {
	t.Helper()
	id := atomic.AddInt64(&timerTestCounter, 1)
	dsn := fmt.Sprintf("file:test_timer_%s_%d?mode=memory&cache=shared", t.Name(), id)
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

	u := &users.User{Email: fmt.Sprintf("tz-%d@x.com", time.Now().UnixNano()), Password: "x", Role: users.RoleUser, NotificationsEnabled: true}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("tz-%d.iptv.example", u.ID), UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	return &timerFixture{
		db:       db,
		repo:     NewRepository(db),
		uid:      u.ID,
		domainID: d.ID,
	}
}

var timerTestCounter int64

func (f *timerFixture) addDevice(token, tz string) devices.Device {
	t := f.t()
	uidP := f.uid
	didP := f.domainID
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
		t.Fatalf("seed device: %v", err)
	}
	f.devices = append(f.devices, d)
	return d
}

func (f *timerFixture) t() *testing.T {
	// Convenience: callers already hold *testing.T.
	return timerTestingT
}

// timerTestingT is a sentinel that lets helpers compile against
// *testing.T without threading it through every signature. Each
// helper that needs it panics if it was not set up properly.
var timerTestingT *testing.T

func (f *timerFixture) service() *Service {
	return NewService(f.repo, &fakePusher{}, &fakeDomainRepo{owned: map[uint][]domains.Domain{
		f.uid: {domainOwnedBy(f.uid, f.domainID)},
	}}, &fakeUserRepo{users: map[uint]*users.User{f.uid: userWith(true)}}, nil)
}

func TestService_CreateSchedule_DeviceLocal_FiresAtDeviceLocalTime(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	mexico := f.addDevice("mex-1", "America/Mexico_City")
	spain := f.addDevice("spa-1", "Europe/Madrid")
	bogota := f.addDevice("bog-1", "America/Bogota")
	noTZ := f.addDevice("ntz-1", "")

	svc := f.service()

	// Cron "0 9 * * *" = "every day at 9:00 local". The test
	// does not depend on the exact UTC moment; it asserts the
	// shape: four timers, four DIFFERENT UTC moments, devices
	// in the same TZ get the same fire time.
	before := time.Now().UTC()
	_, timers, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "news", Body: "9pm local", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 9 * * *", TimezoneModeDeviceLocal, "")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if len(timers) != 4 {
		t.Fatalf("got %d timers, want 4 (one per device)", len(timers))
	}

	byDevice := map[uint]time.Time{}
	for _, tm := range timers {
		byDevice[tm.DeviceID] = tm.FireAt
		if tm.FireAt.Before(before) {
			t.Errorf("timer for device %d fires in the past: %s", tm.DeviceID, tm.FireAt)
		}
	}

	// Devices in distinct TZs must get distinct UTC fire times.
	// The only way they could collide is if robfig/cron ignored
	// the device's Location — which is exactly the regression
	// we want to catch.
	seen := map[string]int{}
	for _, d := range []devices.Device{mexico, spain, bogota, noTZ} {
		fire := byDevice[d.ID].UTC().Format(time.RFC3339)
		seen[fire]++
	}
	if len(seen) < 3 {
		t.Errorf("expected at least 3 distinct fire moments across 4 TZs, got %d: %v", len(seen), seen)
	}

	// Each fire time, viewed in the device's local clock, must
	// read exactly 9:00 (the cron expression).
	for _, d := range []devices.Device{mexico, spain, bogota, noTZ} {
		loc, lerr := time.LoadLocation(d.Timezone)
		if lerr != nil || loc == nil {
			loc = time.UTC
		}
		local := byDevice[d.ID].In(loc)
		if local.Hour() != 9 || local.Minute() != 0 {
			t.Errorf("device %s (tz=%q): local fire = %s, want 09:00 local",
				d.Token, d.Timezone, local.Format(time.RFC3339))
		}
	}
}

func TestService_CreateSchedule_Shared_FiresAtSharedTime(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("mex-2", "America/Mexico_City")
	f.addDevice("spa-2", "Europe/Madrid")

	svc := f.service()
	_, timers, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "show", Body: "21:00 shared", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 21 * * *", TimezoneModeShared, "America/Mexico_City")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if len(timers) != 2 {
		t.Fatalf("got %d timers, want 2", len(timers))
	}

	// Shared mode = all devices fire at the SAME UTC moment.
	if !timers[0].FireAt.Equal(timers[1].FireAt) {
		t.Errorf("shared mode: device 1 fires at %s, device 2 at %s (must match)",
			timers[0].FireAt, timers[1].FireAt)
	}
	// And the fire moment, in the schedule's Timezone, reads 21:00.
	mx, _ := time.LoadLocation("America/Mexico_City")
	if mx == nil {
		t.Skip("America/Mexico_City unavailable in tzdata")
	}
	local := timers[0].FireAt.In(mx)
	if local.Hour() != 21 || local.Minute() != 0 {
		t.Errorf("shared TZ local fire = %s, want 21:00", local.Format(time.RFC3339))
	}
}

func TestService_CreateSchedule_RejectsInvalidTimezone(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-1", "UTC")

	svc := f.service()
	_, _, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "x", Body: "x", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *", TimezoneModeShared, "Not/A/Zone")
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("err = %v, want ErrInvalidSchedule", err)
	}
}

func TestService_CreateSchedule_NoAudienceFails(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	svc := f.service()
	_, _, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "x", Body: "x", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_RECURRING, nil, "0 * * * *", TimezoneModeShared, "UTC")
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("err = %v, want ErrInvalidSchedule", err)
	}
}

func TestService_DispatchTimer_FiresOnce(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-1", "America/Mexico_City")

	svc := f.service()
	// Run_at must be >= 1 minute in the future per
	// validateSchedule's one_shot rule. We still need a fire
	// time that has passed before DispatchTimer can run; the
	// test bypasses the validation by creating the timer row
	// directly with a backdated fire_at.
	sched, timers, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT, ptrTime(time.Now().Add(time.Hour)), "", TimezoneModeShared, "UTC")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if len(timers) != 1 {
		t.Fatalf("want 1 timer, got %d", len(timers))
	}
	timer := timers[0]
	// Backdate the timer so ListDueTimers returns it.
	if err := f.db.Model(&ScheduledPushTimer{}).Where("id = ?", timer.ID).Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate timer: %v", err)
	}
	if err := f.db.First(&timer, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}

	if err := svc.DispatchTimer(context.Background(), sched, timer); err != nil {
		t.Fatalf("DispatchTimer: %v", err)
	}

	var got ScheduledPushTimer
	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.DispatchedAt == nil {
		t.Fatal("DispatchedAt not stamped")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }