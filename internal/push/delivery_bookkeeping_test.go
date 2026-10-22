package push

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/realtime"
	"gorm.io/gorm"
)

func adminPayload() *pb.PushPayload {
	return &pb.PushPayload{
		Category: pb.PushCategory_PUSH_CATEGORY_ADMIN_MESSAGE,
		Title:    "t", Body: "b", Priority: pb.PushPriority_PUSH_PRIORITY_NORMAL,
	}
}

// countDeliveryUpdates counts the UPDATE statements issued against
// delivery_attempts on db from now on.
func countDeliveryUpdates(t *testing.T, db *gorm.DB) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	if err := db.Callback().Update().After("gorm:update").Register("test:count_delivery_updates", func(tx *gorm.DB) {
		if tx.Statement.Table == "delivery_attempts" {
			n.Add(1)
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	return &n
}

func attemptsByState(t *testing.T, db *gorm.DB, pushID uint) map[DeliveryState]map[FailureReason]int {
	t.Helper()
	var rows []DeliveryAttempt
	if err := db.Where("push_message_id = ?", pushID).Find(&rows).Error; err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	out := map[DeliveryState]map[FailureReason]int{}
	for _, r := range rows {
		if out[r.State] == nil {
			out[r.State] = map[FailureReason]int{}
		}
		out[r.State][r.FailureReason]++
	}
	return out
}

// TestService_CreateImmediate_MarksFailuresInBulk proves the failed
// deliveries of one fan-out are recorded with one UPDATE per failure
// reason instead of one UPDATE per device.
func TestService_CreateImmediate_MarksFailuresInBulk(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	pusher := newFakePusher()
	for i, tok := range []string{"bulk-1", "bulk-2", "bulk-3", "bulk-4"} {
		dev := f.addDevice(tok, "UTC")
		if i < 3 {
			pusher.result[uint64(dev.ID)] = realtime.ErrDeviceNotConnected
		} else {
			pusher.result[uint64(dev.ID)] = errors.New("redis down")
		}
	}
	updates := countDeliveryUpdates(t, f.db)

	msg, _, err := f.serviceWithPusher(pusher).CreateImmediate(context.Background(), f.uid, f.uid, []uint{f.domainID}, adminPayload())
	if err != nil {
		t.Fatalf("CreateImmediate: %v", err)
	}

	got := attemptsByState(t, f.db, msg.ID)
	if got[StateFailed][FailureDeviceOffline] != 3 || got[StateFailed][FailureInternalError] != 1 {
		t.Fatalf("attempts = %v, want 3 device_offline and 1 internal_error", got)
	}
	if n := updates.Load(); n != 2 {
		t.Fatalf("delivery_attempts UPDATE statements = %d, want 2 (one per reason)", n)
	}
}

// TestRepository_MarkDeliveriesFailed_OnlyTouchesSentRows keeps the
// per-row guard of the bulk update: a delivered row is never flipped
// to failed.
func TestRepository_MarkDeliveriesFailed_OnlyTouchesSentRows(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("guard", "UTC")
	ctx := context.Background()
	now := time.Now()
	msg := &PushMessage{UserID: f.uid, Category: CategoryEventAlert, Title: "t", Body: "b", Priority: PriorityNormal, Source: SourceImmediate}
	if err := f.repo.InsertPushMessageWithTargets(ctx, msg, []uint{f.domainID}, &now); err != nil {
		t.Fatalf("insert push: %v", err)
	}
	if err := f.repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{
		{PushMessageID: msg.ID, DeviceID: dev.ID, MessageID: "guard-sent", State: StateSent, SentAt: &now},
		{PushMessageID: msg.ID, DeviceID: dev.ID, MessageID: "guard-delivered", State: StateDelivered, SentAt: &now},
	}); err != nil {
		t.Fatalf("insert attempts: %v", err)
	}

	n, err := f.repo.MarkDeliveriesFailed(ctx, []string{"guard-sent", "guard-delivered"}, FailureDeviceOffline)
	if err != nil {
		t.Fatalf("MarkDeliveriesFailed: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}
	got := attemptsByState(t, f.db, msg.ID)
	if got[StateFailed][FailureDeviceOffline] != 1 || got[StateDelivered][""] != 1 {
		t.Fatalf("attempts = %v, want one failed and one delivered", got)
	}
}
