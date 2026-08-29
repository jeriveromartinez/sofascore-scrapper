package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
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
	logger     *slog.Logger
	// messageCounter is a per-process atomic used to mint the
	// transport message_id. It is intentionally not persisted:
	// the message_id is only meaningful to the client, not to the
	// server. The server-side lookup is by (push_id, device_id),
	// which the client supplies via the ack.
	messageCounter atomic.Uint64
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
	// are connected).
	attempts := make([]DeliveryAttempt, 0, len(audience))
	for _, dev := range audience {
		attempts = append(attempts, DeliveryAttempt{
			PushMessageID: msg.ID,
			DeviceID:      dev.ID,
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
		pushFrame := buildWsPush(msg.ID, dev.ID, &now, payload)
		if err := s.pusher.PublishPush(ctx, uint64(dev.ID), pushFrame); err != nil {
			if errors.Is(err, realtime.ErrDeviceNotConnected) {
				if markErr := s.repo.MarkDeliveryFailed(ctx, msg.ID, dev.ID, FailureDeviceOffline); markErr != nil {
					s.logger.Warn("push: mark offline failed",
						slog.Uint64("push_id", uint64(msg.ID)),
						slog.Uint64("device_id", uint64(dev.ID)),
						slog.String("error", markErr.Error()))
				}
				continue
			}
			s.logger.Warn("push: publish failed",
				slog.Uint64("push_id", uint64(msg.ID)),
				slog.Uint64("device_id", uint64(dev.ID)),
				slog.String("error", err.Error()))
			if markErr := s.repo.MarkDeliveryFailed(ctx, msg.ID, dev.ID, FailureInternalError); markErr != nil {
				s.logger.Warn("push: mark internal-error failed",
					slog.Uint64("push_id", uint64(msg.ID)),
					slog.Uint64("device_id", uint64(dev.ID)),
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
// DELIVERED. The messageID is currently unused on the server but
// kept in the signature for future use (e.g. latency histograms
// keyed by message_id).
func (s *Service) OnAck(ctx context.Context, pushID, deviceID, _ uint64) error {
	return s.repo.MarkDeliveryDelivered(ctx, uint(pushID), uint(deviceID), time.Now())
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
// message_id is a monotonic per-process counter; it is not
// persisted because the server identifies deliveries by
// (push_id, device_id). The client uses message_id to display
// latency or to deduplicate retransmits.
func buildWsPush(pushID, deviceID uint, sentAt *time.Time, payload *pb.PushPayload) *pb.WsPush {
	data := map[string]string{}
	for k, v := range payload.GetData() {
		data[k] = v
	}
	var sentAtMS int64
	if sentAt != nil {
		sentAtMS = sentAt.UnixMilli()
	}
	_ = deviceID
	return &pb.WsPush{
		PushId:     uint64(pushID),
		MessageId:  nextMessageID(),
		Category:   payload.GetCategory(),
		Title:      payload.GetTitle(),
		Body:       payload.GetBody(),
		ImageUrl:   payload.GetImageUrl(),
		DeepLink:   payload.GetDeepLink(),
		Priority:   payload.GetPriority(),
		TtlSeconds: payload.GetTtlSeconds(),
		Data:       data,
		SentAt:     sentAtMS,
	}
}
