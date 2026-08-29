package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/redis/go-redis/v9"
)

// PubSubChannel is the Redis channel where push frames are broadcast.
// All backend instances subscribe to the same channel; the
// `push:fanout` name is the spec default. Tests may use a different
// name via SubscriberConfig.Channel to keep messages isolated.
const PubSubChannel = "push:fanout"

// SubscriberConfig parameterizes a Subscriber. Channel defaults to
// PubSubChannel; Logger defaults to slog.Default().
type SubscriberConfig struct {
	Channel string
	Logger  *slog.Logger
}

// Subscriber is the per-backend Redis pub/sub consumer. It subscribes
// to a single fan-out channel and routes every message to the local
// hub. Any error from DeliverToLocal is logged and swallowed: the
// spec requires that we do NOT retry (offline devices are counted
// as device_offline and that's it).
type Subscriber struct {
	redis  *redis.Client
	hub    *Hub
	cfg    SubscriberConfig
	logger *slog.Logger

	startOnce sync.Once
	stopOnce  sync.Once
	done      chan struct{}
	exitHooks []func()
}

// NewSubscriber constructs a Subscriber. The hub is the local
// WebSocket registry; the redis client is the broker connection
// (the same connection used by internal/push to publish).
func NewSubscriber(client *redis.Client, hub *Hub, cfg SubscriberConfig) *Subscriber {
	if cfg.Channel == "" {
		cfg.Channel = PubSubChannel
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Subscriber{
		redis:  client,
		hub:    hub,
		cfg:    cfg,
		logger: cfg.Logger,
		done:   make(chan struct{}),
	}
}

// OnExit registers a hook invoked exactly once when the subscriber's
// goroutine returns. Used by tests to assert clean shutdown.
func (s *Subscriber) OnExit(hook func()) {
	s.exitHooks = append(s.exitHooks, hook)
}

// Start spawns the background goroutine that consumes the channel.
// It is safe to call once; subsequent calls are no-ops.
func (s *Subscriber) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("realtime.Subscriber.Start: nil context")
	}
	var startErr error
	s.startOnce.Do(func() {
		sub := s.redis.Subscribe(ctx, s.cfg.Channel)
		// Wait for confirmation that the subscription is created
		// before publishing anything. Without this, a publish on
		// the same goroutine immediately after Subscribe can race
		// the broker and be lost.
		if _, err := sub.Receive(ctx); err != nil {
			startErr = fmt.Errorf("subscribe %q: %w", s.cfg.Channel, err)
			return
		}
		go s.run(ctx, sub)
	})
	return startErr
}

// Stop signals the goroutine to exit and closes the underlying
// subscription. It is safe to call multiple times; subsequent calls
// are no-ops.
func (s *Subscriber) Stop() {
	s.stopOnce.Do(func() {
		close(s.done)
	})
}

// publishEnvelope is the JSON shape we put on the Redis channel. We
// use JSON (not raw bytes) so the payload is debuggable with
// redis-cli MONITOR and stays friendly to other tooling. The frame
// itself is base64 inside the JSON because proto bytes are not
// always valid UTF-8.
type publishEnvelope struct {
	DeviceID uint64 `json:"device_id"`
	FrameB64 string `json:"frame_b64"`
}

// Publish encodes the WsPush into a frame and broadcasts it to every
// backend subscribed to the channel. Each backend's subscriber
// receives the envelope and forwards to its local hub; the hub
// itself decides whether the device is local.
func (s *Subscriber) Publish(ctx context.Context, deviceID uint64, push *pb.WsPush) error {
	frame := &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: push}}
	raw, err := encodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode push: %w", err)
	}
	env := publishEnvelope{DeviceID: deviceID, FrameB64: encodeBase64(raw)}
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return s.redis.Publish(ctx, s.cfg.Channel, payload).Err()
}

// run is the long-lived consume loop. It blocks on the subscription's
// Receive channel and the shutdown signal. Any delivery error from
// the hub is logged and dropped (we never propagate to other
// backends; the per-device row in delivery_attempts is the source of
// truth for what happened).
func (s *Subscriber) run(ctx context.Context, sub *redis.PubSub) {
	defer func() {
		_ = sub.Close()
		for _, hook := range s.exitHooks {
			hook()
		}
	}()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			s.handle(msg.Payload)
		}
	}
}

// handle parses one envelope and routes it to the local hub. Errors
// are logged with the device_id (when parseable) and never
// propagated: a malformed message must not bring down the consumer.
func (s *Subscriber) handle(raw string) {
	var env publishEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		s.logger.Warn("pubsub: invalid envelope", slog.String("error", err.Error()))
		return
	}
	frameBytes, err := decodeBase64(env.FrameB64)
	if err != nil {
		s.logger.Warn("pubsub: invalid base64",
			slog.Uint64("device_id", env.DeviceID),
			slog.String("error", err.Error()))
		return
	}
	frame, err := decodeFrame(frameBytes)
	if err != nil {
		s.logger.Warn("pubsub: invalid frame",
			slog.Uint64("device_id", env.DeviceID),
			slog.String("error", err.Error()))
		return
	}
	if err := s.hub.DeliverToLocal(env.DeviceID, &rawFramePayload{raw: frameBytes, frame: frame}); err != nil {
		// ErrDeviceNotConnected is the expected case for any device
		// that is connected to a sibling backend. We log at debug
		// to avoid flooding production logs.
		if errors.Is(err, ErrDeviceNotConnected) {
			s.logger.Debug("pubsub: device not local",
				slog.Uint64("device_id", env.DeviceID))
			return
		}
		s.logger.Warn("pubsub: deliver failed",
			slog.Uint64("device_id", env.DeviceID),
			slog.String("error", err.Error()))
	}
}

// rawFramePayload is a FramePayload built from a pre-encoded frame.
// We carry both the raw bytes (for the wire) and the parsed frame
// (for logging), but encode() just returns the raw bytes.
type rawFramePayload struct {
	raw   []byte
	frame *pb.WsFrame
}

func (r *rawFramePayload) encode() ([]byte, error) { return r.raw, nil }
