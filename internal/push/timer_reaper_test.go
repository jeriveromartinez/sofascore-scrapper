package push

import (
	"context"
	"testing"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

// oneShotSchedule creates a one-shot schedule whose timer is already
// due, so a test can drive DispatchTimer directly.
func (f *timerFixture) oneShotSchedule(t *testing.T) (*ScheduledPush, ScheduledPushTimer) {
	t.Helper()
	svc := f.service()
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
	if err := f.db.Model(&ScheduledPushTimer{}).Where("id = ?", timer.ID).
		Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate timer: %v", err)
	}
	if err := f.db.First(&timer, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	return sched, timer
}

// TestService_ReapStaleClaims_RedispatchesCrashedTimer covers the
// durability gap the reaper closes: a worker claimed the timer and
// then died before publishing. The claim is still open, so the timer
// is invisible to the pending query; the reaper releases it and the
// next tick delivers the push.
func TestService_ReapStaleClaims_RedispatchesCrashedTimer(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-crash", "UTC")

	pusher := newFakePusher()
	svc := f.serviceWithPusher(pusher)
	sched, timer := f.oneShotSchedule(t)

	// Simulate the crash: the claim is stamped but the dispatch
	// never happened.
	claimedAt := time.Now().Add(-30 * time.Minute)
	if _, err := f.repo.MarkTimerDispatched(context.Background(), timer.ID, claimedAt); err != nil {
		t.Fatalf("mark dispatched: %v", err)
	}

	due, err := svc.ListDueTimers(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("claimed timer is still due: %d rows", len(due))
	}

	released, err := svc.ReleaseStaleTimerClaims(context.Background(), time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want 1", released)
	}

	due, err = svc.ListDueTimers(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("want the released timer to be due again, got %d rows", len(due))
	}

	if err := svc.DispatchTimer(context.Background(), sched, due[0]); err != nil {
		t.Fatalf("DispatchTimer: %v", err)
	}
	pusher.mu.Lock()
	calls := len(pusher.calls)
	pusher.mu.Unlock()
	if calls != 1 {
		t.Fatalf("published %d times, want 1", calls)
	}
}

// TestService_ReapStaleClaims_KeepsCompletedDispatch is the guard
// against a duplicate-push storm: a dispatch that COMPLETED has its
// claim closed, so the reaper must leave it alone no matter how old
// the cutoff is.
func TestService_ReapStaleClaims_KeepsCompletedDispatch(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-done", "UTC")

	pusher := newFakePusher()
	svc := f.serviceWithPusher(pusher)
	sched, timer := f.oneShotSchedule(t)

	if err := svc.DispatchTimer(context.Background(), sched, timer); err != nil {
		t.Fatalf("DispatchTimer: %v", err)
	}

	var got ScheduledPushTimer
	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.DispatchedAt == nil {
		t.Fatal("dispatched_at not stamped")
	}
	if got.ClaimedAt != nil {
		t.Fatal("claimed_at not cleared after a completed dispatch")
	}

	// A cutoff in the FUTURE: any age-based rule would match this
	// row, so only the "claim was closed" rule keeps it safe.
	released, err := svc.ReleaseStaleTimerClaims(context.Background(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if released != 0 {
		t.Fatalf("released = %d, want 0 (the dispatch completed)", released)
	}

	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.DispatchedAt == nil {
		t.Fatal("a completed dispatch was re-armed")
	}

	due, err := svc.ListDueTimers(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("completed timer is due again: %d rows", len(due))
	}
}

// TestService_ReapStaleClaims_KeepsRecentClaim asserts the reaper
// only releases claims that are older than the stale window: a
// worker that is still dispatching must not have its timer stolen.
func TestService_ReapStaleClaims_KeepsRecentClaim(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-recent", "UTC")

	svc := f.service()
	_, timer := f.oneShotSchedule(t)

	if _, err := f.repo.MarkTimerDispatched(context.Background(), timer.ID, time.Now()); err != nil {
		t.Fatalf("mark dispatched: %v", err)
	}

	released, err := svc.ReleaseStaleTimerClaims(context.Background(), time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if released != 0 {
		t.Fatalf("released = %d, want 0 (the claim is recent)", released)
	}

	var got ScheduledPushTimer
	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.ClaimedAt == nil {
		t.Fatal("a recent claim was released")
	}
	if got.DispatchedAt == nil {
		t.Fatal("a recent claim was released")
	}
}
