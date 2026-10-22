package push

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
)

// serviceWithPusher builds a Service bound to the fixture's repo but
// with a caller-supplied pusher, so a test can count publications.
func (f *timerFixture) serviceWithPusher(p Pusher) *Service {
	return NewService(f.repo, p, &fakeDomainRepo{owned: map[uint][]domains.Domain{
		f.uid: {domainOwnedBy(f.uid, f.domainID)},
	}}, &fakeUserRepo{users: map[uint]*users.User{f.uid: userWith(true)}}, nil)
}

// TestService_DispatchTimer_PublishesOnlyOnce covers the idempotency
// of a scheduled fire: the timer is claimed before the push is
// published, so running the same timer twice publishes once. Before
// the fix the publish happened before the stamp, so a re-run (or a
// crash between publish and stamp) sent the notification again.
func TestService_DispatchTimer_PublishesOnlyOnce(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	f.addDevice("d-1", "UTC")

	pusher := newFakePusher()
	svc := f.serviceWithPusher(pusher)

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
	if err := f.db.Model(&ScheduledPushTimer{}).Where("id = ?", timer.ID).Update("fire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate timer: %v", err)
	}
	if err := f.db.First(&timer, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}

	if err := svc.DispatchTimer(context.Background(), sched, timer); err != nil {
		t.Fatalf("DispatchTimer: %v", err)
	}
	if err := svc.DispatchTimer(context.Background(), sched, timer); err != nil {
		t.Fatalf("DispatchTimer (second run): %v", err)
	}

	pusher.mu.Lock()
	calls := len(pusher.calls)
	pusher.mu.Unlock()
	if calls != 1 {
		t.Fatalf("published %d times, want 1", calls)
	}

	var messages int64
	if err := f.db.Model(&PushMessage{}).Count(&messages).Error; err != nil {
		t.Fatalf("count push messages: %v", err)
	}
	if messages != 1 {
		t.Fatalf("persisted %d push messages, want 1", messages)
	}

	var got ScheduledPushTimer
	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.DispatchedAt == nil {
		t.Fatal("dispatched_at not stamped")
	}
}

// TestService_DispatchTimer_ReleasesClaimOnFailure covers the
// rollback path: a dispatch that fails must leave the timer
// unclaimed so the next tick retries it.
func TestService_DispatchTimer_ReleasesClaimOnFailure(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("d-2", "UTC")

	pusher := newFakePusher()
	pusher.result[uint64(dev.ID)] = errors.New("publish failed")
	svc := f.serviceWithPusher(pusher)

	sched, timers, err := svc.CreateSchedule(context.Background(), f.uid, f.uid, []uint{f.domainID}, &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}, pb.PushScheduleType_PUSH_SCHEDULE_TYPE_ONE_SHOT, ptrTime(time.Now().Add(time.Hour)), "", TimezoneModeShared, "UTC")
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	timer := timers[0]

	if err := svc.DispatchTimer(context.Background(), sched, timer); err == nil {
		t.Fatal("DispatchTimer succeeded, want publish error")
	}

	var got ScheduledPushTimer
	if err := f.db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("re-read timer: %v", err)
	}
	if got.DispatchedAt != nil {
		t.Fatal("timer stayed claimed after a failed dispatch")
	}
}

// TestRepository_MarkDeliveryDelivered_KeepsTerminalState covers the
// state guard on the ack path: an attempt that already failed must
// not be flipped back to delivered by a late ack.
func TestRepository_MarkDeliveryDelivered_KeepsTerminalState(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("d-3", "UTC")
	repo := f.repo
	ctx := context.Background()
	now := time.Now()

	msg := &PushMessage{
		UserID: f.uid, Category: CategoryEventAlert, Title: "t", Body: "b",
		Priority: PriorityNormal, Source: SourceImmediate,
	}
	if err := repo.InsertPushMessageWithTargets(ctx, msg, []uint{f.domainID}, &now); err != nil {
		t.Fatalf("insert push message: %v", err)
	}
	if err := repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{{
		PushMessageID: msg.ID,
		DeviceID:      dev.ID,
		MessageID:     "msg-terminal",
		State:         StateSent,
		SentAt:        &now,
	}}); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if err := repo.MarkDeliveryFailed(ctx, "msg-terminal", FailureDeviceOffline); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	if _, err := repo.MarkDeliveryDelivered(ctx, "msg-terminal", now.Add(time.Second)); !errors.Is(err, ErrInvalidDeliveryState) {
		t.Fatalf("err = %v, want ErrInvalidDeliveryState", err)
	}

	var got DeliveryAttempt
	if err := f.db.First(&got, "message_id = ?", "msg-terminal").Error; err != nil {
		t.Fatalf("re-read attempt: %v", err)
	}
	if got.State != StateFailed {
		t.Fatalf("state = %q, want failed", got.State)
	}
	if got.AckedAt != nil {
		t.Fatal("acked_at set on a failed delivery")
	}
}

// TestRepository_MarkDeliveryDelivered_TransitionsFromSent is the
// happy path: a sent attempt flips to delivered and keeps its
// latency.
func TestRepository_MarkDeliveryDelivered_TransitionsFromSent(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("d-4", "UTC")
	repo := f.repo
	ctx := context.Background()
	now := time.Now()

	msg := &PushMessage{
		UserID: f.uid, Category: CategoryEventAlert, Title: "t", Body: "b",
		Priority: PriorityNormal, Source: SourceImmediate,
	}
	if err := repo.InsertPushMessageWithTargets(ctx, msg, []uint{f.domainID}, &now); err != nil {
		t.Fatalf("insert push message: %v", err)
	}
	if err := repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{{
		PushMessageID: msg.ID,
		DeviceID:      dev.ID,
		MessageID:     "msg-sent",
		State:         StateSent,
		SentAt:        &now,
	}}); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}

	latencyMS, err := repo.MarkDeliveryDelivered(ctx, "msg-sent", now.Add(50*time.Millisecond))
	if err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if latencyMS < 40 || latencyMS > 200 {
		t.Fatalf("latency_ms = %d, want ~50", latencyMS)
	}

	var got DeliveryAttempt
	if err := f.db.First(&got, "message_id = ?", "msg-sent").Error; err != nil {
		t.Fatalf("re-read attempt: %v", err)
	}
	if got.State != StateDelivered {
		t.Fatalf("state = %q, want delivered", got.State)
	}
}
