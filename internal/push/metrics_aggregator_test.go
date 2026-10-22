package push

import (
	"context"
	"fmt"
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
