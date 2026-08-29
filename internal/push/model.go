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
	UserID      uint             `gorm:"index;not null"`
	User        *users.User      `gorm:"foreignKey:UserID"`
	Category    Category         `gorm:"size:32;not null"`
	Title       string           `gorm:"size:200;not null"`
	Body        string           `gorm:"size:2000;not null"`
	ImageURL    string           `gorm:"size:500"`
	DeepLink    string           `gorm:"size:500"`
	Priority    Priority         `gorm:"size:16;not null;default:normal"`
	TTLSeconds  int              `gorm:"not null;default:0"`
	DataJSON    StringJSON       `gorm:"type:json"`        // free-form metadata; nil = empty
	Source      Source           `gorm:"size:16;not null"` // "immediate" | "scheduled"
	ScheduledID *uint            `gorm:"index"`            // non-nil only when Source == scheduled
	Domains     []domains.Domain `gorm:"many2many:push_message_targets;joinForeignKey:push_message_id;joinReferences:domain_id"`
}

// PushMessageTarget is the join table between a push and its
// audience domains. Modeled explicitly so we can add metadata later
// (e.g. "sent at" per target) without a migration.
type PushMessageTarget struct {
	PushMessageID uint `gorm:"primaryKey;not null"`
	DomainID      uint `gorm:"primaryKey;not null"`
}

// ScheduledPush is a push that fires automatically — either at a
// fixed timestamp (one_shot) or on a recurring cron (recurring). The
// next_fire_at column is what the runner polls on; it is updated
// after each fire.
type ScheduledPush struct {
	gorm.Model
	UserID       uint             `gorm:"index;not null"`
	User         *users.User      `gorm:"foreignKey:UserID"`
	ScheduleType ScheduleType     `gorm:"size:16;not null"`
	RunAt        *time.Time       `gorm:"null"`           // one_shot only
	CronExpr     string           `gorm:"size:64"`        // recurring only
	NextFireAt   time.Time        `gorm:"index;not null"` // what the runner polls
	LastFiredAt  *time.Time       `gorm:"null"`
	IsActive     bool             `gorm:"not null;default:true;index"`
	Category     Category         `gorm:"size:32;not null"`
	Title        string           `gorm:"size:200;not null"`
	Body         string           `gorm:"size:2000;not null"`
	ImageURL     string           `gorm:"size:500"`
	DeepLink     string           `gorm:"size:500"`
	Priority     Priority         `gorm:"size:16;not null;default:normal"`
	TTLSeconds   int              `gorm:"not null;default:0"`
	DataJSON     StringJSON       `gorm:"type:json"`
	Domains      []domains.Domain `gorm:"many2many:scheduled_push_targets;joinForeignKey:scheduled_push_id;joinReferences:domain_id"`
}

// ScheduledPushTarget is the join table between a scheduled_push and
// its audience domains.
type ScheduledPushTarget struct {
	ScheduledPushID uint `gorm:"primaryKey;not null"`
	DomainID        uint `gorm:"primaryKey;not null"`
}

// DeliveryAttempt is one row per (push, device) — the universe of
// devices a push was sent to at fire time. Updated as the delivery
// lifecycle progresses: sent → delivered | failed.
type DeliveryAttempt struct {
	gorm.Model
	PushMessageID uint          `gorm:"uniqueIndex:uq_push_device;not null"`
	DeviceID      uint          `gorm:"uniqueIndex:uq_push_device;not null"`
	MessageID     string        `gorm:"size:64;index;not null"` // UUID v4 transport id from the client
	State         DeliveryState `gorm:"size:16;not null;index"`
	FailureReason FailureReason `gorm:"size:32"`
	SentAt        *time.Time    `gorm:"null"`
	AckedAt       *time.Time    `gorm:"null"`
	LatencyMS     *int          `gorm:"null"`
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
