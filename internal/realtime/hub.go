package realtime

import (
	"errors"
	"log/slog"
	"sync"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// ErrDeviceNotConnected is returned by DeliverToLocal when the
// targeted device has no live connection on this backend. The push
// service uses this to record delivery_attempts.state = failed,
// failure_reason = device_offline.
var ErrDeviceNotConnected = errors.New("realtime: device not connected to this backend")

// FramePayload is implemented by anything the hub can encode and
// deliver. *pb.WsFrame satisfies it directly; test fakes can
// implement it to bypass the full proto stack.
type FramePayload interface {
	encode() ([]byte, error)
}

// Hub owns the local table of live WebSocket connections. A given
// backend instance has exactly one Hub; cross-instance delivery is
// the Subscriber's job (see pubsub.go).
//
// The Hub is safe for concurrent use: register, unregister, get,
// count, and deliver can be called from any goroutine.
type Hub struct {
	mu      sync.RWMutex
	conns   map[uint64]*Connection
	metrics interface {
		SetWSConnections(n int)
	}
}

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[uint64]*Connection)}
}

// Register associates a Connection with its device_id. If a
// connection is already registered under the same device_id, the
// old one is closed and evicted; the new one wins. This handles
// the reconnect race (flaky network) where a client opens a new
// socket before the old close handler has run.
func (h *Hub) Register(deviceID uint64, c *Connection) {
	h.mu.Lock()
	if old, ok := h.conns[deviceID]; ok && old != c {
		// Evict the previous connection. The new one takes its
		// place; the old writer/read goroutines exit when the
		// socket close propagates.
		old.Close(1011, "evicted by new connection for same device")
		delete(h.conns, deviceID)
	}
	h.conns[deviceID] = c
	count := len(h.conns)
	h.mu.Unlock()
	h.notifyConnCount(count)
}

// Unregister removes the connection for deviceID and closes it. The
// close is idempotent: if the connection was already closed, the
// second call is a no-op.
func (h *Hub) Unregister(deviceID uint64) {
	h.mu.Lock()
	c, ok := h.conns[deviceID]
	if ok {
		delete(h.conns, deviceID)
	}
	count := len(h.conns)
	h.mu.Unlock()
	if ok && c != nil {
		c.Close(1000, "client closed or hub unregister")
	}
	h.notifyConnCount(count)
}

// SetMetrics wires the gauge callback that fires on every
// register/unregister. The interface (rather than a concrete
// *push.Metrics) avoids an import cycle: realtime would otherwise
// have to import push just to bump a counter.
func (h *Hub) SetMetrics(m interface{ SetWSConnections(n int) }) {
	h.metrics = m
	h.notifyConnCount(h.Count())
}

func (h *Hub) notifyConnCount(n int) {
	if h.metrics != nil {
		h.metrics.SetWSConnections(n)
	}
}

// Get returns the connection registered under deviceID, if any.
func (h *Hub) Get(deviceID uint64) (*Connection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[deviceID]
	return c, ok
}

// Count returns the number of live connections on this backend.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// DeliverToLocal encodes the given frame and pushes it to the
// connection for deviceID if it is connected locally. It returns
// ErrDeviceNotConnected if the device is not on this backend (caller
// must record delivery_attempts.device_offline and rely on a sibling
// instance to deliver).
//
// The frame is encoded synchronously; the actual socket write
// happens in the connection's writer goroutine, so this method
// returns as soon as the bytes are enqueued on the send channel.
func (h *Hub) DeliverToLocal(deviceID uint64, payload FramePayload) error {
	c, ok := h.Get(deviceID)
	if !ok {
		return ErrDeviceNotConnected
	}
	raw, err := payload.encode()
	if err != nil {
		return err
	}
	// Non-blocking send: if the writer is behind, the connection is
	// probably dead. We close it and report the failure so the
	// delivery is recorded as ws_disconnected, not silently dropped.
	select {
	case c.send <- raw:
		return nil
	default:
		// Send buffer saturated — log it so operators can correlate
		// with a slow client (the most common cause). The push
		// service will mark the row as device_offline; the warning
		// here is the only signal that the disconnect was triggered
		// by backpressure rather than the socket dying on its own.
		if c.logger != nil {
			c.logger.Warn("realtime: ws send buffer full, closing connection",
				slog.Uint64("device_id", c.deviceID),
				slog.Int("buffer_cap", cap(c.send)))
		}
		c.Close(1011, "send buffer full")
		return ErrDeviceNotConnected
	}
}

// DeliverPushToLocal is a typed convenience wrapper around
// DeliverToLocal for the common case of a WsPush.
func (h *Hub) DeliverPushToLocal(deviceID uint64, push *pb.WsPush) error {
	return h.DeliverToLocal(deviceID, &wsPushPayload{push: push})
}

// wsPushPayload adapts *pb.WsPush to the FramePayload interface.
type wsPushPayload struct {
	push *pb.WsPush
}

func (w *wsPushPayload) encode() ([]byte, error) {
	return encodeFrame(&pb.WsFrame{Payload: &pb.WsFrame_Push{Push: w.push}})
}
