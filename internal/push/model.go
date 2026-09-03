// Package push contains the GORM models, repositories, services, and
// REST handlers for the push-notifications feature. The package is
// the domain layer; transport (WebSocket hub) lives in
// internal/realtime and the cron runner is wired into internal/scheduler.
package push

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// Category mirrors the PushCategory proto enum as a string column.
// Stored as a varchar (not a SQL enum) so adding new categories does
// not require a migration. Validated at the service layer.
type Category string

const (
	CategoryEventAlert   Category = "event_alert"
	CategoryApkUpdate    Category = "apk_update"
	CategoryAdminMessage Category = "admin_message"
	CategoryScheduled    Category = "scheduled"
)

func (c Category) Valid() bool {
	switch c {
	case CategoryEventAlert, CategoryApkUpdate, CategoryAdminMessage, CategoryScheduled:
		return true
	}
	return false
}

// Priority mirrors the PushPriority proto enum. Default Normal.
type Priority string

const (
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

func (p Priority) Valid() bool {
	return p == PriorityNormal || p == PriorityHigh
}

// ScheduleType discriminates between one-shot timestamps and recurring
// cron expressions on a scheduled_push row.
type ScheduleType string

const (
	ScheduleTypeOneShot   ScheduleType = "one_shot"
	ScheduleTypeRecurring ScheduleType = "recurring"
)

func (s ScheduleType) Valid() bool {
	return s == ScheduleTypeOneShot || s == ScheduleTypeRecurring
}

// Source discriminates whether a PushMessage originated from an
// immediate REST call or from a scheduled_push firing.
type Source string

const (
	SourceImmediate Source = "immediate"
	SourceScheduled Source = "scheduled"
)

// DeliveryState is the lifecycle state of a single delivery_attempts
// row, mirroring the DeliveryState proto enum.
type DeliveryState string

const (
	StateSent      DeliveryState = "sent"      // WS WriteMessage OK, waiting for ack
	StateDelivered DeliveryState = "delivered" // client acked
	StateFailed    DeliveryState = "failed"    // timeout, disconnect, or error
)

func (s DeliveryState) Valid() bool {
	switch s {
	case StateSent, StateDelivered, StateFailed:
		return true
	}
	return false
}

// FailureReason explains why a delivery attempt did not succeed.
// Mirrors the DeliveryFailureReason proto enum.
type FailureReason string

const (
	FailureDeviceOffline  FailureReason = "device_offline"
	FailureSendTimeout    FailureReason = "send_timeout"
	FailureWSDisconnected FailureReason = "ws_disconnected"
	FailureDomainMismatch FailureReason = "domain_mismatch"
	FailureExpiredToken   FailureReason = "expired_token"
	FailureInternalError  FailureReason = "internal_error"
)

// PushMessage is the header row of a single push delivery. The
// payload fields are denormalized (not a foreign key to a separate
// "templates" table) because they never change after send and we want
// zero joins at delivery time.
type PushMessage struct {
	gorm.Model
	UserID      uint        `gorm:"column:user_id;not null;index:idx_push_messages_user_created,priority:1;foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	User        *users.User `gorm:"foreignKey:UserID;references:ID"`
	Category    Category    `gorm:"column:category;size:32;not null"`
	Title       string      `gorm:"column:title;size:200;not null"`
	Body        string      `gorm:"column:body;size:2000;not null"`
	ImageURL    string      `gorm:"column:image_url;size:500"`
	Priority    Priority    `gorm:"column:priority;size:16;not null;default:'normal'"`
	TTLSeconds  int         `gorm:"column:ttl_seconds;not null;default:0"`
	DataJSON    StringJSON  `gorm:"column:data_json;type:json"`
	Source      Source      `gorm:"column:source;size:16;not null"`
	ScheduledID *uint       `gorm:"column:scheduled_id;null;foreignKey:ScheduledID;references:ID;constraint:OnDelete:SET_NULL"`
	CreatedAt   time.Time   `gorm:"column:created_at;not null;index:idx_push_messages_user_created,priority:2"`
}

// PushMessageTarget is the join table between a push and its
// audience domains. Modeled explicitly so we can add metadata later
// (e.g. "sent at" per target) without a migration.
type PushMessageTarget struct {
	PushMessageID uint          `gorm:"column:push_message_id;primaryKey;not null;foreignKey:PushMessageID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	DomainID      uint          `gorm:"column:domain_id;primaryKey;not null;foreignKey:DomainID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	PushMessage   *PushMessage  `gorm:"foreignKey:PushMessageID;references:ID"`
	Domain        *domains.Domain `gorm:"foreignKey:DomainID;references:ID"`
}

// TimezoneMode discriminates how the cron expression of a
// ScheduledPush is interpreted. SHARED (default) evaluates the
// cron once in the schedule's Timezone and fires the whole
// audience at the resulting UTC moment. DEVICE_LOCAL evaluates
// the cron per device in each device's registered timezone,
// producing a separate fire time per device (each with its own
// row in scheduled_push_timers).
type TimezoneMode string

const (
	TimezoneModeShared     TimezoneMode = "shared"
	TimezoneModeDeviceLocal TimezoneMode = "device_local"
)

func (m TimezoneMode) Valid() bool {
	return m == TimezoneModeShared || m == TimezoneModeDeviceLocal
}

// ScheduledPush is a push that fires automatically — either at a
// fixed timestamp (one_shot) or on a recurring cron (recurring).
// Fire times are stored per device in scheduled_push_timers; this
// row is the campaign config (audience, payload, schedule). The
// NextFireAt column mirrors the earliest pending timer across the
// schedule's audience — the worker keeps it in sync on enqueue
// and on fire, so the existing dashboard column keeps working.
type ScheduledPush struct {
	gorm.Model
	UserID          uint         `gorm:"column:user_id;not null;foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	User            *users.User  `gorm:"foreignKey:UserID;references:ID"`
	ScheduleType    ScheduleType `gorm:"column:schedule_type;size:16;not null"`
	RunAt           *time.Time   `gorm:"column:run_at;null"`
	CronExpr        string       `gorm:"column:cron_expr;size:64"`
	NextFireAt      time.Time    `gorm:"column:next_fire_at;not null;index:idx_schedules_active_next,priority:2"`
	LastFiredAt     *time.Time   `gorm:"column:last_fired_at;null"`
	IsActive        bool         `gorm:"column:is_active;not null;default:true;index:idx_schedules_active_next,priority:1"`
	TimezoneMode    TimezoneMode `gorm:"column:timezone_mode;size:16;not null;default:'shared'"`
	Timezone        string       `gorm:"column:timezone;size:64;not null;default:'UTC'"`
	Category        Category     `gorm:"column:category;size:32;not null"`
	Title           string       `gorm:"column:title;size:200;not null"`
	Body            string       `gorm:"column:body;size:2000;not null"`
	ImageURL        string       `gorm:"column:image_url;size:500"`
	Priority        Priority     `gorm:"column:priority;size:16;not null;default:'normal'"`
	TTLSeconds      int          `gorm:"column:ttl_seconds;not null;default:0"`
	DataJSON        StringJSON   `gorm:"column:data_json;type:json"`
}

// ScheduledPushTimer is the per-device fire row for a scheduled
// push. There is one row per audience device per pending fire; once
// the timer fires, DispatchedAt is stamped and the row stays for
// audit. For recurring schedules the worker inserts a new row with
// the next fire time after each dispatch.
//
// This is the durable source of truth for "when does each device
// receive this schedule". Redis ZSETs cache the index but are
// rebuilt from this table on startup.
type ScheduledPushTimer struct {
	gorm.Model
	ScheduledPushID uint      `gorm:"column:scheduled_push_id;not null;index:idx_spt_schedule_fire,priority:1;foreignKey:ScheduledPushID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	ScheduledPush   *ScheduledPush `gorm:"foreignKey:ScheduledPushID;references:ID"`
	DeviceID        uint      `gorm:"column:device_id;not null;index:idx_spt_device_dispatched,priority:1;foreignKey:DeviceID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	FireAt          time.Time `gorm:"column:fire_at;not null;index:idx_spt_schedule_fire,priority:2"`
	DispatchedAt    *time.Time `gorm:"column:dispatched_at;null;index:idx_spt_device_dispatched,priority:2"`
}

// ScheduledPushTarget is the join table between a scheduled_push and
// its audience domains.
type ScheduledPushTarget struct {
	ScheduledPushID uint            `gorm:"column:scheduled_push_id;primaryKey;not null;foreignKey:ScheduledPushID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	DomainID        uint            `gorm:"column:domain_id;primaryKey;not null;foreignKey:DomainID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	ScheduledPush   *ScheduledPush  `gorm:"foreignKey:ScheduledPushID;references:ID"`
	Domain          *domains.Domain `gorm:"foreignKey:DomainID;references:ID"`
}

// DeliveryAttempt is one row per (push, device) — the universe of
// devices a push was sent to at fire time. Updated as the delivery
// lifecycle progresses: sent → delivered | failed.
type DeliveryAttempt struct {
	gorm.Model
	PushMessageID uint          `gorm:"column:push_message_id;not null;index:idx_attempts_push_message,priority:1;foreignKey:PushMessageID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	DeviceID      uint          `gorm:"column:device_id;not null;index:idx_attempts_device_created,priority:1;foreignKey:DeviceID;references:ID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	MessageID     string        `gorm:"column:message_id;size:36;uniqueIndex:uq_message_id;not null"`
	State         DeliveryState `gorm:"column:state;size:16;not null;index:idx_attempts_state_created,priority:1"`
	FailureReason FailureReason `gorm:"column:failure_reason;size:32;null"`
	SentAt        *time.Time    `gorm:"column:sent_at;null"`
	AckedAt       *time.Time    `gorm:"column:acked_at;null"`
	LatencyMS     *int          `gorm:"column:latency_ms;null"`
	CreatedAt     time.Time     `gorm:"column:created_at;not null;index:idx_attempts_state_created,priority:2;index:idx_attempts_device_created,priority:2;index:idx_attempts_push_message,priority:2"`
}

// StringJSON is a nullable map[string]string stored as a JSON column.
// Used for the free-form `data` field of a push. The zero value
// (nil) marshals to SQL NULL and back to a nil map.
type StringJSON map[string]string

// Value implements driver.Valuer. A nil map writes SQL NULL; a
// non-nil map is JSON-encoded.
func (s StringJSON) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(map[string]string(s))
}

// Scan implements sql.Scanner. Empty string or NULL scans to nil;
// a JSON object decodes into the map.
func (s *StringJSON) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		if len(v) == 0 {
			*s = nil
			return nil
		}
		var m map[string]string
		if err := json.Unmarshal(v, &m); err != nil {
			return err
		}
		*s = m
		return nil
	case string:
		if v == "" {
			*s = nil
			return nil
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return err
		}
		*s = m
		return nil
	}
	return errors.New("push.StringJSON: unsupported scan source type")
}
