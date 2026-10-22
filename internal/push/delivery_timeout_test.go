package push

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func failedTotal(t *testing.T, g prometheus.Gatherer, reason FailureReason) float64 {
	t.Helper()
	families, err := g.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "push_failed_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "reason" && l.GetValue() == string(reason) {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// TestService_ReleaseStaleTimerClaims_FailsUnackedDeliveries covers
// deliveries that no instance ever acknowledges: the cross-instance
// pusher cannot report an offline device, so without the timeout the
// row would stay sent forever. The scheduler tick fails rows older
// than the ack window and leaves fresh and terminal rows alone.
func TestService_ReleaseStaleTimerClaims_FailsUnackedDeliveries(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("stale", "UTC")
	ctx := context.Background()
	now := time.Now()
	stale := now.Add(-deliveryAckWindow - time.Minute)
	msg := &PushMessage{UserID: f.uid, Category: CategoryEventAlert, Title: "t", Body: "b", Priority: PriorityNormal, Source: SourceImmediate}
	if err := f.repo.InsertPushMessageWithTargets(ctx, msg, []uint{f.domainID}, &now); err != nil {
		t.Fatalf("insert push: %v", err)
	}
	if err := f.repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{
		{PushMessageID: msg.ID, DeviceID: dev.ID, MessageID: "stale-sent", State: StateSent, SentAt: &stale},
		{PushMessageID: msg.ID, DeviceID: dev.ID, MessageID: "fresh-sent", State: StateSent, SentAt: &now},
		{PushMessageID: msg.ID, DeviceID: dev.ID, MessageID: "stale-delivered", State: StateDelivered, SentAt: &stale},
	}); err != nil {
		t.Fatalf("insert attempts: %v", err)
	}
	reg := prometheus.NewRegistry()
	svc := f.service()
	svc.SetMetrics(NewMetrics(reg))
	before := failedTotal(t, reg, FailureSendTimeout)

	if _, err := svc.ReleaseStaleTimerClaims(ctx, now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("ReleaseStaleTimerClaims: %v", err)
	}

	want := map[string]struct {
		state  DeliveryState
		reason FailureReason
	}{
		"stale-sent":      {StateFailed, FailureSendTimeout},
		"fresh-sent":      {StateSent, ""},
		"stale-delivered": {StateDelivered, ""},
	}
	for messageID, w := range want {
		var got DeliveryAttempt
		if err := f.db.First(&got, "message_id = ?", messageID).Error; err != nil {
			t.Fatalf("read %s: %v", messageID, err)
		}
		if got.State != w.state || got.FailureReason != w.reason {
			t.Errorf("%s = %s/%q, want %s/%q", messageID, got.State, got.FailureReason, w.state, w.reason)
		}
	}
	if delta := failedTotal(t, reg, FailureSendTimeout) - before; delta != 1 {
		t.Errorf("push_failed_total{reason=send_timeout} grew by %v, want 1", delta)
	}
}
