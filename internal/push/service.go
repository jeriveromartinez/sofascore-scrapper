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
	logger  *slog.Logger
	metrics *Metrics
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

// Repo exposes the underlying repository. Used by the scheduler
// runner to fetch join-table rows (target domains) before calling
// DispatchScheduled. Kept narrow so callers don't start relying
// on it for business-logic paths.
func (s *Service) Repo() *Repository {
	return s.repo
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

	category, title, body, imageURL, deepLink, priority, ttlSeconds, data, _ := PayloadFromProto(payload)
	now := time.Now()
	msg := &PushMessage{
		UserID:     ownerID,
		Category:   category,
		Title:      title,
		Body:       body,
		ImageURL:   imageURL,
		DeepLink:   deepLink,
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
				continue
			}
			s.logger.Warn("push: publish failed",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureInternalError); markErr != nil {
				s.logger.Warn("push: mark internal-error failed",
					slog.String("message_id", pushFrames[dev.ID].MessageId),
					slog.String("error", markErr.Error()))
			}
		}
	}
	return msg, deviceIDs, nil
}

// CreateSchedule validates and persists a scheduled_push. It does
// not fire anything (the cron runner does that). Returns the
// persisted schedule with the domain associations loaded.
func (s *Service) CreateSchedule(ctx context.Context, callerID, ownerID uint, domainIDs []uint, payload *pb.PushPayload, scheduleType pb.PushScheduleType, runAt *time.Time, cronExpr string) (*ScheduledPush, error) {
	if err := s.authorize(ctx, callerID, ownerID); err != nil {
		return nil, err
	}
	if err := s.validateFeatureEnabled(ctx, ownerID); err != nil {
		return nil, err
	}
	if err := s.validateDomains(ctx, ownerID, domainIDs); err != nil {
		return nil, err
	}
	if err := s.validatePayload(payload); err != nil {
		return nil, err
	}
	if err := s.validateSchedule(scheduleType, runAt, cronExpr); err != nil {
		return nil, err
	}

	category, title, body, imageURL, deepLink, priority, ttlSeconds, data, _ := PayloadFromProto(payload)
	now := time.Now()
	sched := &ScheduledPush{
		UserID:     ownerID,
		Category:   category,
		Title:      title,
		Body:       body,
		ImageURL:   imageURL,
		DeepLink:   deepLink,
		Priority:   priority,
		TTLSeconds: ttlSeconds,
		DataJSON:   data,
		NextFireAt: now,
		IsActive:   true,
	}
	switch ScheduleTypeFromProto(scheduleType) {
	case ScheduleTypeOneShot:
		sched.ScheduleType = ScheduleTypeOneShot
		sched.RunAt = runAt
		sched.NextFireAt = *runAt
	case ScheduleTypeRecurring:
		sched.ScheduleType = ScheduleTypeRecurring
		sched.CronExpr = cronExpr
		parsed, err := s.parser.Parse(cronExpr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSchedule, err)
		}
		sched.NextFireAt = parsed.Next(now)
	}
	if err := s.repo.InsertScheduledPushWithTargets(ctx, sched, domainIDs); err != nil {
		return nil, fmt.Errorf("insert schedule: %w", err)
	}
	return sched, nil
}

// OnAck is the entry point for the realtime hub. It is called from
// the connection's reader loop when the client echoes a
// WsPushAck. We flip the matching delivery_attempts row to
// DELIVERED. The messageID is the client-side transport UUID v4
// (Tasks 8-9) that uniquely identifies the delivery attempt.
func (s *Service) OnAck(ctx context.Context, messageID string) error {
	return s.repo.MarkDeliveryDelivered(ctx, messageID, time.Now())
}

// DispatchScheduled fires a single scheduled push. It is called
// from the scheduler runner for every row that is due
// (is_active = true and next_fire_at <= now). The flow is the
// same as CreateImmediate's: persist a push_messages row, snapshot
// delivery_attempts, publish to the hub, and record per-device
// outcomes. The schedule is then either deactivated (one_shot) or
// rescheduled (recurring).
//
// The caller (scheduler runner) is responsible for fetching the
// schedule's target domainIDs from the join table and passing them
// in. This keeps the schedule struct free of m2m preloads.
//
// Unlike CreateImmediate, this method does NOT re-validate the
// per-user feature toggle or domain ownership. The schedule is
// the source of truth: if the user disabled notifications after
// creating the schedule, the existing schedule still fires (the
// spec's "se desactiva en cascada" rule is enforced at toggle
// time, not at fire time). The audience filter still runs and the
// rows are still snapshotted, so a deactivated user can
// reactivate and immediately see the metrics.
func (s *Service) DispatchScheduled(ctx context.Context, schedule *ScheduledPush, domainIDs []uint) error {
	now := time.Now()
	msg := &PushMessage{
		UserID:      schedule.UserID,
		Category:    schedule.Category,
		Title:       schedule.Title,
		Body:        schedule.Body,
		ImageURL:    schedule.ImageURL,
		DeepLink:    schedule.DeepLink,
		Priority:    schedule.Priority,
		TTLSeconds:  schedule.TTLSeconds,
		DataJSON:    schedule.DataJSON,
		Source:      SourceScheduled,
		ScheduledID: &schedule.ID,
	}
	if err := s.repo.InsertPushMessageWithTargets(ctx, msg, domainIDs, &now); err != nil {
		return fmt.Errorf("insert scheduled push: %w", err)
	}
	if err := s.markScheduleFired(ctx, schedule, now); err != nil {
		s.logger.Warn("push: mark schedule fired",
			slog.Uint64("schedule_id", uint64(schedule.ID)),
			slog.String("error", err.Error()))
	}
	payload := payloadFromSchedule(schedule)
	if err := s.dispatchToAudience(ctx, msg, audienceFilter{DomainIDs: domainIDs}, payload); err != nil {
		return fmt.Errorf("dispatch: %w", err)
	}
	return nil
}

// markScheduleFired handles the per-type post-fire state. For
// one_shot, it deactivates the row. For recurring, it computes
// the next fire and stamps last_fired_at.
func (s *Service) markScheduleFired(ctx context.Context, schedule *ScheduledPush, now time.Time) error {
	switch schedule.ScheduleType {
	case ScheduleTypeOneShot:
		return s.repo.MarkScheduledPushFired(ctx, schedule.ID, now)
	case ScheduleTypeRecurring:
		parsed, err := s.parser.Parse(schedule.CronExpr)
		if err != nil {
			return fmt.Errorf("parse cron: %w", err)
		}
		next := parsed.Next(now)
		return s.repo.RescheduleRecurring(ctx, schedule.ID, next, now)
	default:
		return fmt.Errorf("unknown schedule_type %q", schedule.ScheduleType)
	}
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
				continue
			}
			s.logger.Warn("push: publish failed",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			if markErr := s.repo.MarkDeliveryFailed(ctx, pushFrames[dev.ID].MessageId, FailureInternalError); markErr != nil {
				s.logger.Warn("push: mark internal-error failed",
					slog.String("message_id", pushFrames[dev.ID].MessageId),
					slog.String("error", markErr.Error()))
			}
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
		DeepLink:   s.DeepLink,
		Priority:   PriorityToProto(s.Priority),
		TtlSeconds: int32(s.TTLSeconds),
		Data:       map[string]string(s.DataJSON),
	}
}

// ListDueScheduledPushes returns schedules whose next_fire_at has
// arrived. Exposed as a service method so the scheduler runner in
// internal/scheduler does not need to import internal/push.
func (s *Service) ListDueScheduledPushes(ctx context.Context, now time.Time, limit int) ([]ScheduledPush, error) {
	return s.repo.ListDueScheduledPushes(ctx, now, limit)
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
	_, _, _, _, _, _, _, _, ok := PayloadFromProto(p)
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
func buildWsPush(pushID uint64, payload *pb.PushPayload) *pb.WsPush {
	return &pb.WsPush{
		PushId:    pushID,
		MessageId: nextMessageID(),
		Category:  payload.GetCategory(),
		Title:     payload.GetTitle(),
		Body:      payload.GetBody(),
		Data:      payload.GetData(),
		SentAt:    time.Now().UnixMilli(),
	}
}
