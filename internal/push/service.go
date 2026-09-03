package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	redisplatform "github.com/jeriveromartinez/sofascore-scrapper/internal/platform/redis"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Domain-level errors that handlers map to HTTP status codes. The
// string is what the operator sees; the sentinel is what callers
// switch on.
var (
	ErrForbidden       = errors.New("push: forbidden")
	ErrNotFound        = errors.New("push: not found")
	ErrInvalidPayload  = errors.New("push: invalid payload")
	ErrDisabledFeature = errors.New("push: notifications disabled for user")
	ErrInvalidSchedule = errors.New("push: invalid schedule")
)

// Pusher is the interface the service uses to deliver a push to
// the realtime hub. The concrete realtime.Subscriber satisfies
// it; tests inject a fake to assert dispatch without a broker.
type Pusher interface {
	PublishPush(ctx context.Context, deviceID uint64, push *pb.WsPush) error
}

// DomainLister is the minimal interface the service needs from the
// domains package to validate that every requested domain_id
// belongs to the caller. The concrete domains.Repository satisfies
// it.
type DomainLister interface {
	ListByUser(ctx context.Context, userID uint) ([]domains.Domain, error)
}

// UserGetter is the minimal interface the service needs from the
// users package to load the user (for the notifications_enabled
// gate) and audit who fired a push.
type UserGetter interface {
	GetByID(id uint) (*users.User, error)
}

// Service is the business-logic layer of the push feature. It
// validates inputs, persists the push (immediate or scheduled),
// resolves the audience, and dispatches the frame to the realtime
// hub. It is also the entry point for ack handling: the hub's
// reader loop calls OnAck when a WsPushAck arrives.
type Service struct {
	repo       *Repository
	pusher     Pusher
	domainRepo DomainLister
	userRepo   UserGetter
	parser     cron.Parser
	logger     *slog.Logger
	metrics    *Metrics
}

// NewService wires a Service. All dependencies are required.
func NewService(repo *Repository, pusher Pusher, domainRepo DomainLister, userRepo UserGetter, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:       repo,
		pusher:     pusher,
		domainRepo: domainRepo,
		userRepo:   userRepo,
		parser:     cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		logger:     logger,
	}
}

// SetMetrics wires the Prometheus metrics. nil-safe at the call
// sites; the service continues to work without metrics.
func (s *Service) SetMetrics(m *Metrics) {
	s.metrics = m
}

// GetScheduledPushTargets returns the join rows linking a schedule
// to its target domains. Exposed so callers (notably the scheduler
// push runner) don't have to reach through Repo() to fetch them.
func (s *Service) GetScheduledPushTargets(ctx context.Context, schedID uint) ([]ScheduledPushTarget, error) {
	return s.repo.GetScheduledPushTargets(ctx, schedID)
}

// CreateImmediate validates the request, persists a push_messages
// row + delivery_attempts snapshot, and publishes a WsPush to every
// audience device over the realtime hub.
//
// Returns the created PushMessage and the audience size (number of
// devices that were targeted). Devices that are not currently
// connected are still snapshotted into delivery_attempts with
// state=DEVICE_OFFLINE because the audience filter runs before the
// hub lookup.
func (s *Service) CreateImmediate(ctx context.Context, callerID, ownerID uint, domainIDs []uint, payload *pb.PushPayload) (*PushMessage, []uint, error) {
	if err := s.authorize(ctx, callerID, ownerID); err != nil {
		return nil, nil, err
	}
	if err := s.validateFeatureEnabled(ctx, ownerID); err != nil {
		return nil, nil, err
	}
	if err := s.validateDomains(ctx, ownerID, domainIDs); err != nil {
		return nil, nil, err
	}
	if err := s.validatePayload(payload); err != nil {
		return nil, nil, err
	}

	category, title, body, imageURL, priority, ttlSeconds, data, _ := PayloadFromProto(payload)
	now := time.Now()
	msg := &PushMessage{
		UserID:     ownerID,
		Category:   category,
		Title:      title,
		Body:       body,
		ImageURL:   imageURL,
		Priority:   priority,
		TTLSeconds: ttlSeconds,
		DataJSON:   data,
		Source:     SourceImmediate,
	}
	if err := s.repo.InsertPushMessageWithTargets(ctx, msg, domainIDs, &now); err != nil {
		return nil, nil, fmt.Errorf("insert push: %w", err)
	}

	// Audience: devices whose domain_id is in the requested set.
	// We snapshot the universe regardless of whether the device
	// is currently online; offline ones are recorded as
	// not_delivered: device_offline at metric time.
	audience, err := s.repo.ListAudienceDevicesForDomains(ctx, domainIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list audience: %w", err)
	}
	deviceIDs := deviceIDsOf(audience)

	// Insert delivery_attempts (SENT for everyone; the actual
	// socket write happens immediately below for the ones that
	// are connected). We build the push frames first to capture
	// the transport message_id (stored as a string on each row).
	pushFrames := make(map[uint]*pb.WsPush, len(audience))
	attempts := make([]DeliveryAttempt, 0, len(audience))
	for _, dev := range audience {
		frame := buildWsPush(uint64(msg.ID), payload)
		pushFrames[dev.ID] = frame
		attempts = append(attempts, DeliveryAttempt{
			PushMessageID: msg.ID,
			DeviceID:      dev.ID,
			MessageID:     frame.MessageId,
			State:         StateSent,
			SentAt:        &now,
		})
		s.metrics.IncDispatched(string(SourceImmediate))
	}
	if err := s.repo.InsertDeliveryAttempts(ctx, attempts); err != nil {
		return nil, nil, fmt.Errorf("insert delivery attempts: %w", err)
	}

	// Dispatch to the hub. The hub returns ErrDeviceNotConnected
	// for siblings; we record that as the failure_reason on the
	// attempt row so the metric snapshot is accurate.
	for _, dev := range audience {
		if err := s.pusher.PublishPush(ctx, uint64(dev.ID), pushFrames[dev.ID]); err != nil {
			if errors.Is(err, realtime.ErrDeviceNotConnected) {
				if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureDeviceOffline); markErr != nil {
					s.logger.Warn("push: mark offline failed",
						slog.String("message_id", pushFrames[dev.ID].MessageId),
						slog.String("error", markErr.Error()))
				}
				s.metrics.IncFailed(FailureDeviceOffline)
				s.logger.Info("push: ws delivery skipped",
					slog.Uint64("push_id", uint64(msg.ID)),
					slog.Uint64("device_id", uint64(dev.ID)),
					slog.String("reason", string(FailureDeviceOffline)))
				continue
			}
			s.logger.Warn("push: ws publish failed",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureInternalError); markErr != nil {
				s.logger.Warn("push: mark internal-error failed",
					slog.String("message_id", pushFrames[dev.ID].MessageId),
					slog.String("error", markErr.Error()))
			}
			s.metrics.IncFailed(FailureInternalError)
		} else {
			s.logger.Info("push: ws publish queued",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("message_id", pushFrames[dev.ID].MessageId))
		}
	}
	return msg, deviceIDs, nil
}

// CreateSchedule validates and persists a scheduled_push along
// with one timer per audience device. The cron expression is
// evaluated per device when timezoneMode == DEVICE_LOCAL (each device
// fires at its own local wall-clock moment) and once in the
// schedule's Timezone when timezoneMode == SHARED (all devices fire
// at the same UTC moment).
//
// The caller (handler) is responsible for enqueuing the returned
// timers into the TimerStore so the worker can pick them up. We
// split this because the handler is the boundary that knows whether
// Redis is available; falling back to a DB scan is the worker's
// job, not the service's.
func (s *Service) CreateSchedule(ctx context.Context, callerID, ownerID uint, domainIDs []uint, payload *pb.PushPayload, scheduleType pb.PushScheduleType, runAt *time.Time, cronExpr string, tzMode TimezoneMode, timezone string) (*ScheduledPush, []ScheduledPushTimer, error) {
	if err := s.authorize(ctx, callerID, ownerID); err != nil {
		return nil, nil, err
	}
	if err := s.validateFeatureEnabled(ctx, ownerID); err != nil {
		return nil, nil, err
	}
	if err := s.validateDomains(ctx, ownerID, domainIDs); err != nil {
		return nil, nil, err
	}
	if err := s.validatePayload(payload); err != nil {
		return nil, nil, err
	}
	if err := s.validateSchedule(scheduleType, runAt, cronExpr); err != nil {
		return nil, nil, err
	}
	if !tzMode.Valid() {
		return nil, nil, fmt.Errorf("%w: timezone_mode must be shared or device_local", ErrInvalidSchedule)
	}
	if tzMode == TimezoneModeShared && timezone == "" {
		timezone = "UTC"
	}
	if tzMode == TimezoneModeShared {
		if _, err := time.LoadLocation(timezone); err != nil {
			return nil, nil, fmt.Errorf("%w: invalid timezone %q", ErrInvalidSchedule, timezone)
		}
	}

	category, title, body, imageURL, priority, ttlSeconds, data, _ := PayloadFromProto(payload)
	now := time.Now()
	sched := &ScheduledPush{
		UserID:       ownerID,
		Category:     category,
		Title:        title,
		Body:         body,
		ImageURL:     imageURL,
		Priority:     priority,
		TTLSeconds:   ttlSeconds,
		DataJSON:     data,
		IsActive:     true,
		TimezoneMode: tzMode,
		Timezone:     timezone,
		NextFireAt:   now,
	}

	var parsed cron.Schedule
	switch ScheduleTypeFromProto(scheduleType) {
	case ScheduleTypeOneShot:
		sched.ScheduleType = ScheduleTypeOneShot
		sched.RunAt = runAt
	case ScheduleTypeRecurring:
		sched.ScheduleType = ScheduleTypeRecurring
		sched.CronExpr = cronExpr
		p, err := s.parser.Parse(cronExpr)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrInvalidSchedule, err)
		}
		parsed = p
	}

	// Resolve the audience once so we can compute per-device fire
	// times. The audience must exist on the DB before timers can
	// reference their device_ids.
	devices, err := s.repo.ListAudienceDevicesForDomains(ctx, domainIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list audience: %w", err)
	}
	if len(devices) == 0 {
		return nil, nil, fmt.Errorf("%w: no audience devices in the selected domains", ErrInvalidSchedule)
	}

	timers, earliest, err := s.buildTimers(sched, parsed, runAt, now, devices)
	if err != nil {
		return nil, nil, err
	}
	sched.NextFireAt = earliest

	if err := s.repo.InsertScheduledPushWithTargets(ctx, sched, domainIDs, timers); err != nil {
		return nil, nil, fmt.Errorf("insert schedule: %w", err)
	}
	return sched, timers, nil
}

// buildTimers computes one ScheduledPushTimer per audience device,
// applying the schedule's TimezoneMode. Returns the earliest fire
// time so the caller can stamp ScheduledPush.NextFireAt.
func (s *Service) buildTimers(sched *ScheduledPush, parsed cron.Schedule, runAt *time.Time, now time.Time, audience []devices.Device) ([]ScheduledPushTimer, time.Time, error) {
	if len(audience) == 0 {
		return nil, time.Time{}, fmt.Errorf("buildTimers: empty audience")
	}
	timers := make([]ScheduledPushTimer, 0, len(audience))
	earliest := time.Time{}

	switch sched.ScheduleType {
	case ScheduleTypeOneShot:
		// Single UTC fire for everyone, regardless of TZ mode.
		// The operator picked the moment; clients display it in
		// their local TZ.
		fire := *runAt
		for _, d := range audience {
			timers = append(timers, ScheduledPushTimer{
				DeviceID: d.ID,
				FireAt:   fire,
			})
		}
		earliest = fire
	case ScheduleTypeRecurring:
		for _, d := range audience {
			fire, err := s.nextCronFire(parsed, sched.TimezoneMode, sched.Timezone, d.Timezone, now)
			if err != nil {
				return nil, time.Time{}, err
			}
			timers = append(timers, ScheduledPushTimer{
				DeviceID: d.ID,
				FireAt:   fire,
			})
			if earliest.IsZero() || fire.Before(earliest) {
				earliest = fire
			}
		}
	default:
		return nil, time.Time{}, fmt.Errorf("unknown schedule_type %q", sched.ScheduleType)
	}
	return timers, earliest, nil
}

// nextCronFire evaluates the cron expression in the right TZ and
// returns the next moment >= now. For SHARED mode the schedule's
// Timezone is used; for DEVICE_LOCAL the device's own TZ (falling
// back to UTC if empty or invalid, with a debug log).
func (s *Service) nextCronFire(parsed cron.Schedule, mode TimezoneMode, scheduleTZ, deviceTZ string, now time.Time) (time.Time, error) {
	switch mode {
	case TimezoneModeShared:
		loc, err := time.LoadLocation(scheduleTZ)
		if err != nil || loc == nil {
			loc = time.UTC
		}
		localNow := now.In(loc)
		return parsed.Next(localNow), nil
	case TimezoneModeDeviceLocal:
		loc, err := time.LoadLocation(deviceTZ)
		if err != nil || loc == nil {
			if s.logger != nil {
				s.logger.Debug("push: device has no valid TZ, falling back to UTC for cron eval",
					slog.String("device_tz", deviceTZ))
			}
			loc = time.UTC
		}
		localNow := now.In(loc)
		return parsed.Next(localNow), nil
	default:
		return time.Time{}, fmt.Errorf("unknown timezone_mode %q", mode)
	}
}

// OnAck is the entry point for the realtime hub. It is called from
// the connection's reader loop when the client echoes a
// WsPushAck. We flip the matching delivery_attempts row to
// DELIVERED. The messageID is the client-side transport UUID v4
// (Tasks 8-9) that uniquely identifies the delivery attempt.
func (s *Service) OnAck(ctx context.Context, messageID string) error {
	ackedAt := time.Now()
	latencyMS, err := s.repo.MarkDeliveryDelivered(ctx, messageID, ackedAt)
	if err != nil {
		s.logger.Warn("push: ack persistence failed",
			slog.String("message_id", messageID),
			slog.String("error", err.Error()))
		return err
	}
	s.metrics.IncDelivered()
	s.metrics.ObserveLatency(latencyMS)
	s.logger.Info("push: ws delivery acked",
		slog.String("message_id", messageID),
		slog.Int("latency_ms", latencyMS))
	return nil
}

// DispatchTimer fires ONE scheduled push to the single device that
// owns the timer. The worker calls this once per pending timer it
// pops off the Redis index (or the DB fallback). For recurring
// schedules it computes and inserts the next per-device timer so
// the next firing is also correct.
//
// Like CreateImmediate, this persists a push_messages row +
// delivery_attempts snapshot and publishes a WsPush. The schedule's
// user-facing fields are unchanged.
//
// Two workers racing on the same row is harmless: the DB-side
// dispatched_at guard in MarkTimerDispatched ensures only one
// dispatch goes through. The losing worker just sees RowsAffected=0
// and exits silently.
func (s *Service) DispatchTimer(ctx context.Context, schedule *ScheduledPush, timer ScheduledPushTimer) error {
	now := time.Now()

	// Load the device. We need its TZ to compute the next fire
	// time when the schedule is recurring.
	dev, err := s.repo.FindDeviceByID(ctx, timer.DeviceID)
	if err != nil {
		return fmt.Errorf("load device: %w", err)
	}

	msg := &PushMessage{
		UserID:      schedule.UserID,
		Category:    schedule.Category,
		Title:       schedule.Title,
		Body:        schedule.Body,
		ImageURL:    schedule.ImageURL,
		Priority:    schedule.Priority,
		TTLSeconds:  schedule.TTLSeconds,
		DataJSON:    schedule.DataJSON,
		Source:      SourceScheduled,
		ScheduledID: &schedule.ID,
	}
	domainIDs, err := s.repo.GetScheduledPushDomainIDs(ctx, schedule.ID)
	if err != nil {
		return fmt.Errorf("load domains: %w", err)
	}
	if err := s.repo.InsertPushMessageWithTargets(ctx, msg, domainIDs, &now); err != nil {
		return fmt.Errorf("insert scheduled push: %w", err)
	}

	payload := payloadFromSchedule(schedule)
	if err := s.dispatchToOneDevice(ctx, msg, dev, payload); err != nil {
		return fmt.Errorf("dispatch to device: %w", err)
	}

	// Stamp dispatched_at. Idempotent under worker races.
	stamped, err := s.repo.MarkTimerDispatched(ctx, timer.ID, now)
	if err != nil {
		s.logger.Warn("push: mark timer dispatched failed",
			slog.Uint64("timer_id", uint64(timer.ID)),
			slog.String("error", err.Error()))
	} else if !stamped {
		s.logger.Info("push: timer already dispatched by another worker, skipping post-fire work",
			slog.Uint64("timer_id", uint64(timer.ID)))
		return nil
	}

	// For recurring, compute and insert the next per-device timer.
	if schedule.ScheduleType == ScheduleTypeRecurring {
		if err := s.scheduleNextRecurringFire(ctx, schedule, dev, now); err != nil {
			s.logger.Warn("push: schedule next recurring fire failed",
				slog.Uint64("schedule_id", uint64(schedule.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
		}
	}
	return nil
}

// scheduleNextRecurringFire computes the next per-device fire
// time and inserts a new pending timer row. Updates the schedule's
// NextFireAt column if this fire time is the new earliest. The
// caller is expected to also enqueue the new timer into the
// Redis index (the worker does this on its next tick by reading
// the new DB row).
func (s *Service) scheduleNextRecurringFire(ctx context.Context, schedule *ScheduledPush, dev devices.Device, now time.Time) error {
	parsed, err := s.parser.Parse(schedule.CronExpr)
	if err != nil {
		return fmt.Errorf("parse cron: %w", err)
	}
	next, err := s.nextCronFire(parsed, schedule.TimezoneMode, schedule.Timezone, dev.Timezone, now)
	if err != nil {
		return err
	}
	if err := s.repo.InsertTimer(ctx, &ScheduledPushTimer{
		ScheduledPushID: schedule.ID,
		DeviceID:        dev.ID,
		FireAt:          next,
	}); err != nil {
		return err
	}
	// Bump the schedule's NextFireAt if this row is now the
	// earliest pending timer. Cheap: take MIN over pending
	// timers. Skipped on error to avoid masking the insert error.
	earliest, err := s.repo.EarliestPendingTimer(ctx, schedule.ID)
	if err == nil && !earliest.IsZero() {
		_ = s.repo.UpdateScheduledPushNextFireAt(ctx, schedule.ID, earliest)
	}
	return nil
}

// dispatchToOneDevice is the per-device version of
// dispatchToAudience: snapshot the single delivery_attempt row,
// publish the WsPush, and record the outcome.
func (s *Service) dispatchToOneDevice(ctx context.Context, msg *PushMessage, dev devices.Device, payload *pb.PushPayload) error {
	now := time.Now()
	frame := buildWsPush(uint64(msg.ID), payload)
	if err := s.repo.InsertDeliveryAttempts(ctx, []DeliveryAttempt{{
		PushMessageID: msg.ID,
		DeviceID:      dev.ID,
		MessageID:     frame.MessageId,
		State:         StateSent,
		SentAt:        &now,
	}}); err != nil {
		return fmt.Errorf("insert delivery attempt: %w", err)
	}
	s.metrics.IncDispatched(string(msg.Source))
	if err := s.pusher.PublishPush(ctx, uint64(dev.ID), frame); err != nil {
		if errors.Is(err, realtime.ErrDeviceNotConnected) {
			if markErr := s.repo.MarkDeliveryFailed(ctx, frame.MessageId, FailureDeviceOffline); markErr != nil {
				s.logger.Warn("push: mark offline failed",
					slog.String("message_id", frame.MessageId),
					slog.String("error", markErr.Error()))
			}
			s.metrics.IncFailed(FailureDeviceOffline)
			s.logger.Info("push: ws delivery skipped",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("reason", string(FailureDeviceOffline)))
			return nil
		}
		s.logger.Warn("push: ws publish failed",
			slog.Uint64("push_id", uint64(msg.ID)),
			slog.Uint64("device_id", uint64(dev.ID)),
			slog.String("error", err.Error()))
		if markErr := s.repo.MarkDeliveryFailed(ctx, frame.MessageId, FailureInternalError); markErr != nil {
			s.logger.Warn("push: mark internal-error failed",
				slog.String("message_id", frame.MessageId),
				slog.String("error", markErr.Error()))
		}
		s.metrics.IncFailed(FailureInternalError)
		return err
	}
	s.logger.Info("push: ws publish queued",
		slog.Uint64("push_id", uint64(msg.ID)),
		slog.Uint64("device_id", uint64(dev.ID)),
		slog.String("message_id", frame.MessageId))
	return nil
}

// audienceFilter is a small helper struct for the audience filter
// path. Today it only carries domainIDs; future filters (platform,
// app version, last-seen-recently) extend it.
type audienceFilter struct {
	DomainIDs []uint
}

// dispatchToAudience is the shared path used by both
// CreateImmediate and DispatchScheduled. It snapshots the
// audience, inserts delivery_attempts, publishes each frame to
// the hub, and records per-device outcomes.
func (s *Service) dispatchToAudience(ctx context.Context, msg *PushMessage, filter audienceFilter, payload *pb.PushPayload) error {
	audience, err := s.repo.ListAudienceDevicesForDomains(ctx, filter.DomainIDs)
	if err != nil {
		return fmt.Errorf("list audience: %w", err)
	}
	now := time.Now()
	// Build push frames first so we can capture the transport
	// message_id (stored as a string on each delivery_attempt row).
	pushFrames := make(map[uint]*pb.WsPush, len(audience))
	attempts := make([]DeliveryAttempt, 0, len(audience))
	for _, dev := range audience {
		frame := buildWsPush(uint64(msg.ID), payload)
		pushFrames[dev.ID] = frame
		attempts = append(attempts, DeliveryAttempt{
			PushMessageID: msg.ID,
			DeviceID:      dev.ID,
			MessageID:     frame.MessageId,
			State:         StateSent,
			SentAt:        &now,
		})
		s.metrics.IncDispatched(string(msg.Source))
	}
	if err := s.repo.InsertDeliveryAttempts(ctx, attempts); err != nil {
		return fmt.Errorf("insert delivery attempts: %w", err)
	}
	for _, dev := range audience {
		if err := s.pusher.PublishPush(ctx, uint64(dev.ID), pushFrames[dev.ID]); err != nil {
			if errors.Is(err, realtime.ErrDeviceNotConnected) {
				if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureDeviceOffline); markErr != nil {
					s.logger.Warn("push: mark offline failed",
						slog.String("message_id", pushFrames[dev.ID].MessageId),
						slog.String("error", markErr.Error()))
				}
				s.metrics.IncFailed(FailureDeviceOffline)
				s.logger.Info("push: ws delivery skipped",
					slog.Uint64("push_id", uint64(msg.ID)),
					slog.Uint64("device_id", uint64(dev.ID)),
					slog.String("reason", string(FailureDeviceOffline)))
				continue
			}
			s.logger.Warn("push: ws publish failed",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureInternalError); markErr != nil {
				s.logger.Warn("push: mark internal-error failed",
					slog.String("message_id", pushFrames[dev.ID].MessageId),
					slog.String("error", markErr.Error()))
			}
			s.metrics.IncFailed(FailureInternalError)
		} else {
			s.logger.Info("push: ws publish queued",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("message_id", pushFrames[dev.ID].MessageId))
		}
	}
	return nil
}

// payloadFromSchedule converts a schedule's denormalized payload
// fields into a pb.PushPayload suitable for buildWsPush.
func payloadFromSchedule(s *ScheduledPush) *pb.PushPayload {
	return &pb.PushPayload{
		Category:   CategoryToProto(s.Category),
		Title:      s.Title,
		Body:       s.Body,
		ImageUrl:   s.ImageURL,
		Priority:   PriorityToProto(s.Priority),
		TtlSeconds: int32(s.TTLSeconds),
		Data:       map[string]string(s.DataJSON),
	}
}

// ListDueTimers returns pending timers whose fire_at has arrived,
// joined with their schedule for context. Exposed so the scheduler
// worker does not need to import the push package directly.
func (s *Service) ListDueTimers(ctx context.Context, now time.Time, limit int) ([]ScheduledPushTimer, error) {
	return s.repo.ListDueTimers(ctx, now, limit)
}

// LoadSchedule fetches a schedule by id for the worker. Returns
// ErrRecordNotFound when the row does not exist.
func (s *Service) LoadSchedule(ctx context.Context, id uint) (*ScheduledPush, error) {
	var s2 ScheduledPush
	if err := s.repo.db.WithContext(ctx).First(&s2, id).Error; err != nil {
		return nil, err
	}
	return &s2, nil
}

// EarliestPendingTimer delegates to the repository. The runner
// uses it to keep the schedule header column in sync after a
// recurring fire.
func (s *Service) EarliestPendingTimer(ctx context.Context, scheduleID uint) (time.Time, error) {
	return s.repo.EarliestPendingTimer(ctx, scheduleID)
}

// PendingTimersAfter returns every undispatched timer at or after
// the given moment, projected as the canonical Redis TimerEntry
// shape. The runner uses it to re-enqueue the schedule into Redis
// after a fire.
func (s *Service) PendingTimersAfter(ctx context.Context, scheduleID uint, min time.Time) ([]redisplatform.TimerEntry, error) {
	rows, err := s.repo.PendingTimersAtOrAfter(ctx, scheduleID, min)
	if err != nil {
		return nil, err
	}
	out := make([]redisplatform.TimerEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, redisplatform.TimerEntry{DeviceID: r.DeviceID, FireAt: r.FireAt})
	}
	return out, nil
}

// authorize checks that the caller is either the user themselves
// or holds the admin role.
func (s *Service) authorize(ctx context.Context, callerID, ownerID uint) error {
	if callerID == ownerID {
		return nil
	}
	caller, err := s.userRepo.GetByID(callerID)
	if err != nil {
		return fmt.Errorf("load caller: %w", err)
	}
	if !caller.IsAdmin() {
		return ErrForbidden
	}
	return nil
}

// validateFeatureEnabled rejects the request when the owner has
// not opted in. This is the gate the spec calls "puede habilitarse
// esta seccion de forma dinamica a cada usuario".
func (s *Service) validateFeatureEnabled(ctx context.Context, ownerID uint) error {
	u, err := s.userRepo.GetByID(ownerID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load user: %w", err)
	}
	if !u.NotificationsEnabled {
		return ErrDisabledFeature
	}
	return nil
}

// validateDomains rejects the request when any of the requested
// domain_ids is not owned by the user. This is the gate the spec
// calls "los usuarios nada mas podran enviarles push notifications
// a las apps registradas en su dominio".
func (s *Service) validateDomains(ctx context.Context, ownerID uint, domainIDs []uint) error {
	if len(domainIDs) == 0 {
		return fmt.Errorf("%w: at least one domain_id is required", ErrInvalidPayload)
	}
	owned, err := s.domainRepo.ListByUser(ctx, ownerID)
	if err != nil {
		return fmt.Errorf("list owned domains: %w", err)
	}
	ownedIDs := make(map[uint]bool, len(owned))
	for _, d := range owned {
		ownedIDs[d.ID] = true
	}
	for _, id := range domainIDs {
		if !ownedIDs[id] {
			return fmt.Errorf("%w: domain_id %d is not owned by user %d", ErrForbidden, id, ownerID)
		}
	}
	return nil
}

// validatePayload applies the same rules as the proto layer's
// PayloadFromProto (UNSPECIFIED category/priority, TTL bounds)
// and adds the "title and body are non-empty" rule.
func (s *Service) validatePayload(p *pb.PushPayload) error {
	if p == nil {
		return fmt.Errorf("%w: nil payload", ErrInvalidPayload)
	}
	_, _, _, _, _, _, _, ok := PayloadFromProto(p)
	if !ok {
		return fmt.Errorf("%w: category, priority, or ttl out of range", ErrInvalidPayload)
	}
	if p.Title == "" || p.Body == "" {
		return fmt.Errorf("%w: title and body are required", ErrInvalidPayload)
	}
	if len(p.Title) > 200 || len(p.Body) > 2000 {
		return fmt.Errorf("%w: title or body exceeds length cap", ErrInvalidPayload)
	}
	return nil
}

// validateSchedule enforces the spec rules: ONE_SHOT requires a
// future run_at at least one minute out; RECURRING requires a
// parseable 5-field cron expression.
func (s *Service) validateSchedule(scheduleType pb.PushScheduleType, runAt *time.Time, cronExpr string) error {
	switch ScheduleTypeFromProto(scheduleType) {
	case ScheduleTypeOneShot:
		if runAt == nil {
			return fmt.Errorf("%w: run_at is required for one_shot", ErrInvalidSchedule)
		}
		if runAt.Before(time.Now().Add(time.Minute)) {
			return fmt.Errorf("%w: run_at must be at least one minute in the future", ErrInvalidSchedule)
		}
	case ScheduleTypeRecurring:
		if cronExpr == "" {
			return fmt.Errorf("%w: cron_expr is required for recurring", ErrInvalidSchedule)
		}
		if _, err := s.parser.Parse(cronExpr); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSchedule, err)
		}
	default:
		return fmt.Errorf("%w: schedule_type must be ONE_SHOT or RECURRING", ErrInvalidSchedule)
	}
	return nil
}

// deviceIDsOf flattens a slice of devices to their IDs.
func deviceIDsOf(devs []devices.Device) []uint {
	out := make([]uint, 0, len(devs))
	for _, d := range devs {
		out = append(out, d.ID)
	}
	return out
}

// buildWsPush constructs the on-the-wire WsPush frame. The
// message_id is a globally unique UUID v4 string. It is persisted
// in delivery_attempts.message_id (UNIQUE) and echoed by the
// client in WsPushAck.
//
// Every field carried by pb.PushPayload must be mapped: clients
// (e.g. the Flutter app) gate their local notification rendering
// on priority/image_url/deep_link, so dropping any of them silently
// hides the push from the user even though the bytes were delivered
// on the socket. (Bug fix: B1.)
func buildWsPush(pushID uint64, payload *pb.PushPayload) *pb.WsPush {
	return &pb.WsPush{
		PushId:     pushID,
		MessageId:  nextMessageID(),
		Category:   payload.GetCategory(),
		Title:      payload.GetTitle(),
		Body:       payload.GetBody(),
		ImageUrl:   payload.GetImageUrl(),
		Priority:   payload.GetPriority(),
		TtlSeconds: payload.GetTtlSeconds(),
		Data:       payload.GetData(),
		SentAt:     time.Now().UnixMilli(),
	}
}
