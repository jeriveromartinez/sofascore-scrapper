package push

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

// aggregateFixture is a user with one domain, two live devices, one
// soft-deleted device and one push delivered to the two live devices.
type aggregateFixture struct {
	userID uint
	sentAt time.Time
}

func seedAggregateFixture(t *testing.T, db *gorm.DB) aggregateFixture {
	t.Helper()
	tag := time.Now().UnixNano()
	u := &users.User{Email: fmt.Sprintf("agg-%d@x.com", tag), Password: "x", Role: users.RoleUser, NotificationsEnabled: true}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: fmt.Sprintf("agg-%d.iptv.example", tag), UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	devs := make([]devices.Device, 3)
	for i := range devs {
		uid, did := u.ID, d.ID
		devs[i] = devices.Device{UserID: &uid, DomainID: &did, Token: fmt.Sprintf("agg-%d-%d", tag, i), Platform: "android", Version: "1.0"}
		if err := db.Create(&devs[i]).Error; err != nil {
			t.Fatalf("seed device %d: %v", i, err)
		}
	}
	if err := db.Delete(&devs[2]).Error; err != nil {
		t.Fatalf("soft-delete device: %v", err)
	}

	sentAt := time.Now().Truncate(time.Second)
	msg := &PushMessage{UserID: u.ID, Category: CategoryAdminMessage, Title: "t", Body: "b", Priority: PriorityNormal, Source: SourceImmediate}
	if err := db.Create(msg).Error; err != nil {
		t.Fatalf("seed push: %v", err)
	}
	for i, dev := range devs[:2] {
		if err := db.Create(&DeliveryAttempt{
			PushMessageID: msg.ID,
			DeviceID:      dev.ID,
			MessageID:     fmt.Sprintf("agg-%d-%d", tag, i),
			State:         StateSent,
			SentAt:        &sentAt,
		}).Error; err != nil {
			t.Fatalf("seed attempt %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		db.Unscoped().Where("push_message_id = ?", msg.ID).Delete(&DeliveryAttempt{})
		db.Unscoped().Delete(msg)
		db.Unscoped().Where("domain_id = ?", d.ID).Delete(&devices.Device{})
		db.Unscoped().Delete(d)
		db.Unscoped().Delete(u)
	})
	return aggregateFixture{userID: u.ID, sentAt: sentAt}
}

func assertAggregateSnapshot(t *testing.T, db *gorm.DB, f aggregateFixture) {
	t.Helper()
	snap, err := buildAggregateSnapshot(context.Background(), db, f.userID)
	if err != nil {
		t.Fatalf("buildAggregateSnapshot: %v", err)
	}
	if snap.AudienceSize != 2 {
		t.Errorf("AudienceSize = %d, want 2 (soft-deleted device excluded)", snap.AudienceSize)
	}
	if snap.AudiencePeakToday != 2 {
		t.Errorf("AudiencePeakToday = %d, want 2", snap.AudiencePeakToday)
	}
	if len(snap.TopPlatforms) != 1 || snap.TopPlatforms[0].Platform != "android" || snap.TopPlatforms[0].Count != 2 {
		t.Errorf("TopPlatforms = %v, want android=2", snap.TopPlatforms)
	}
	if snap.LastPushAt == "" {
		t.Error("LastPushAt is empty")
	}
	if len(snap.HourlyHistogram_30D) != 1 {
		t.Fatalf("HourlyHistogram_30D = %v, want one bucket", snap.HourlyHistogram_30D)
	}
	bucket := snap.HourlyHistogram_30D[0]
	if bucket.Count != 2 {
		t.Errorf("hourly bucket count = %d, want 2", bucket.Count)
	}
	if int(bucket.Hour) != f.sentAt.UTC().Hour() {
		t.Errorf("hourly bucket hour = %d, want %d", bucket.Hour, f.sentAt.UTC().Hour())
	}
}

func TestBuildAggregateSnapshot_SQLite(t *testing.T) {
	f := newTimerFixture(t)
	assertAggregateSnapshot(t, f.db, seedAggregateFixture(t, f.db))
}

// TestBuildCampaignSnapshot_LatencyAndFailures covers the latency
// percentiles and the failure breakdown, both computed from the
// delivery_attempts rows of one push.
func TestBuildCampaignSnapshot_LatencyAndFailures(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	dev := f.addDevice("campaign", "UTC")
	now := time.Now()
	msg := &PushMessage{UserID: f.uid, Category: CategoryAdminMessage, Title: "t", Body: "b", Priority: PriorityNormal, Source: SourceImmediate}
	if err := f.db.Create(msg).Error; err != nil {
		t.Fatalf("seed push: %v", err)
	}
	latency := func(ms int) *int { return &ms }
	attempts := []DeliveryAttempt{
		{MessageID: "c-1", State: StateDelivered, LatencyMS: latency(100)},
		{MessageID: "c-2", State: StateDelivered, LatencyMS: latency(200)},
		{MessageID: "c-3", State: StateFailed, FailureReason: FailureSendTimeout},
	}
	for i := range attempts {
		attempts[i].PushMessageID = msg.ID
		attempts[i].DeviceID = dev.ID
		attempts[i].SentAt = &now
	}
	if err := f.db.Create(&attempts).Error; err != nil {
		t.Fatalf("seed attempts: %v", err)
	}

	snap, err := buildCampaignSnapshot(context.Background(), f.db, msg.ID, f.uid)
	if err != nil {
		t.Fatalf("buildCampaignSnapshot: %v", err)
	}
	if snap.LatencyP50Ms != 200 || snap.LatencyP95Ms != 200 {
		t.Errorf("latency p50/p95 = %d/%d, want 200/200", snap.LatencyP50Ms, snap.LatencyP95Ms)
	}
	if len(snap.FailuresByReason) != 1 || snap.FailuresByReason[0].Reason != failureReasonToProto(string(FailureSendTimeout)) || snap.FailuresByReason[0].Count != 1 {
		t.Errorf("failures_by_reason = %v, want one send_timeout", snap.FailuresByReason)
	}
}

// TestBuildAggregateSnapshot_LogsBreakdownErrors proves a failing
// breakdown query is reported instead of silently yielding an empty
// list. The user owns no domain, so the audience query (which also
// reads devices) is skipped and only the top-platforms query fails.
func TestBuildAggregateSnapshot_LogsBreakdownErrors(t *testing.T) {
	f := newTimerFixture(t)
	timerTestingT = t
	if err := f.db.Exec("DROP TABLE devices").Error; err != nil {
		t.Fatalf("drop devices: %v", err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	if _, err := buildAggregateSnapshot(context.Background(), f.db, f.uid+1000); err != nil {
		t.Fatalf("buildAggregateSnapshot: %v", err)
	}
	if !strings.Contains(logs.String(), "push: top platforms query failed") {
		t.Fatalf("logs = %q, want the top platforms query failure", logs.String())
	}
}
