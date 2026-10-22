package push

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

var gaugesTestCounter int64

// newGaugesDB returns an in-memory SQLite DB with the tables the
// gauge counters read from.
func newGaugesDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := atomic.AddInt64(&gaugesTestCounter, 1)
	dsn := fmt.Sprintf("file:test_gauges_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &ScheduledPush{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// gaugeValue reads a gauge from the registry the push metrics are
// registered with.
func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		metrics := f.GetMetric()
		if len(metrics) == 0 {
			t.Fatalf("%s has no metrics", name)
		}
		return metrics[0].GetGauge().GetValue()
	}
	t.Fatalf("%s missing from the registry", name)
	return 0
}

// TestRefreshGaugesPublishesRealCounts proves the two push gauges are
// driven by the database: the enabled-user count only counts users
// with the toggle on, and the schedule count only counts schedules
// that are active and not soft-deleted.
func TestRefreshGaugesPublishesRealCounts(t *testing.T) {
	db := newGaugesDB(t)

	seedUser := func(email string, enabled bool, deleted bool) {
		t.Helper()
		u := &users.User{Email: email, Password: "x", Role: users.RoleUser, NotificationsEnabled: enabled}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		if deleted {
			if err := db.Delete(u).Error; err != nil {
				t.Fatalf("delete user %s: %v", email, err)
			}
		}
	}
	seedUser("on@x.com", true, false)
	seedUser("off@x.com", false, false)
	seedUser("deleted@x.com", true, true)

	seedSchedule := func(active bool, deleted bool) {
		t.Helper()
		s := &ScheduledPush{
			UserID:       1,
			ScheduleType: ScheduleTypeOneShot,
			Category:     CategoryAdminMessage,
			Title:        "t",
			Body:         "b",
			Priority:     PriorityNormal,
			NextFireAt:   time.Now(),
			IsActive:     true,
			TimezoneMode: TimezoneModeShared,
			Timezone:     "UTC",
		}
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("seed schedule: %v", err)
		}
		// The column carries default:true, so GORM inserts a false
		// zero value as the default. Pausing has to go through an
		// explicit UPDATE.
		if !active {
			if err := db.Model(&ScheduledPush{}).Where("id = ?", s.ID).
				Update("is_active", false).Error; err != nil {
				t.Fatalf("pause schedule: %v", err)
			}
		}
		if deleted {
			if err := db.Delete(s).Error; err != nil {
				t.Fatalf("delete schedule: %v", err)
			}
		}
	}
	seedSchedule(true, false)
	seedSchedule(true, false)
	seedSchedule(false, false)
	seedSchedule(true, true)

	reg := prometheus.NewRegistry()
	svc := NewService(NewRepository(db), nil, nil, nil, nil)
	svc.SetMetrics(NewMetrics(reg))

	if err := svc.RefreshGauges(context.Background()); err != nil {
		t.Fatalf("RefreshGauges: %v", err)
	}

	if got := gaugeValue(t, reg, "push_enabled_users"); got != 1 {
		t.Errorf("push_enabled_users = %v, want 1", got)
	}
	if got := gaugeValue(t, reg, "push_active_schedules"); got != 2 {
		t.Errorf("push_active_schedules = %v, want 2", got)
	}
}

// TestRefreshGaugesWithoutMetricsIsSafe proves the refresher is a
// no-op when the push metrics were never wired, instead of panicking
// on a nil collector.
func TestRefreshGaugesWithoutMetricsIsSafe(t *testing.T) {
	db := newGaugesDB(t)
	svc := NewService(NewRepository(db), nil, nil, nil, nil)
	if err := svc.RefreshGauges(context.Background()); err != nil {
		t.Fatalf("RefreshGauges: %v", err)
	}
}
