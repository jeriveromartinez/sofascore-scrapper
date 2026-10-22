package realtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// ackQueueSize bounds the number of acknowledgements buffered per
	// connection. Acks beyond it are dropped: the delivery stays in
	// the sent state, which is the same outcome as a lost ack.
	ackQueueSize = 64
	// ackTimeout bounds the database round trip of a single
	// acknowledgement.
	ackTimeout = 10 * time.Second
)

// AckHandler is invoked when the client sends a WsPushAck back to the
// server. The implementation in internal/push uses this to flip a
// delivery_attempts row from SENT to DELIVERED. messageID is the
// client-side transport UUID v4 that uniquely identifies the delivery
// attempt (future Tasks 8-9); the server looks up the row by it.
//
// The ctx bounds the handler with ackTimeout and is NOT tied to the
// socket lifetime: an acknowledgement that reached the server must be
// persisted even when the client disconnects right after sending it.
type AckHandler func(ctx context.Context, messageID string)

// CloseHandler is invoked exactly once when the connection is being
// torn down (either by the server, by the client, or because of an
// error). It must be safe to call from any goroutine. It receives the
// closing connection so the hub can tell whether it is still the one
// registered for the device.
type CloseHandler func(c *Connection, reason string)

// Connection wraps a single WebSocket. It owns:
//   - the gorilla socket pointer (may be nil in unit tests)
//   - a buffered send channel the writer goroutine drains
//   - the close lifecycle (idempotent Close, observable closedFlag)
//
// One Connection is created per upgrade; the hub holds it for as
// long as the client keeps the socket open.
type Connection struct {
	deviceID uint64
	userID   uint32
	domainID uint32

	ws *websocket.Conn

	send chan []byte

	closedMu   sync.Mutex
	closed     chan struct{}
	closedFlag bool

	ackHandler AckHandler
	closeHook  CloseHandler
	// ackQueue buffers the acknowledgements received from the client
	// so the reader goroutine never blocks on the ack handler, which
	// performs a database round trip.
	ackQueue chan string
	// logger is used for WS send-path diagnostics (e.g. the hub
	// logs when the send buffer saturates and a connection is
	// force-closed). nil-safe: callers may pass nil in unit tests
	// and the hub will skip the log line.
	logger *slog.Logger
}

// newConnection constructs a Connection. ws may be nil in unit tests
// that exercise the hub without a real socket.
func newConnection(deviceID uint64, userID, domainID uint32, ws *websocket.Conn, ack AckHandler, closeHook CloseHandler, logger *slog.Logger) *Connection {
	conn := &Connection{
		deviceID:   deviceID,
		userID:     userID,
		domainID:   domainID,
		ws:         ws,
		send:       make(chan []byte, 16),
		closed:     make(chan struct{}),
		ackHandler: ack,
		closeHook:  closeHook,
		ackQueue:   make(chan string, ackQueueSize),
		logger:     logger,
	}
	go conn.ackLoop()
	return conn
}

// DeviceID exposes the device_id for logging and metrics.
func (c *Connection) DeviceID() uint64 { return c.deviceID }

// UserID exposes the user_id (used for the audience filter and the
// Prometheus audience_size gauge).
func (c *Connection) UserID() uint32 { return c.userID }

// DomainID exposes the device's domain_id (used for the audience
// filter in the push service).
func (c *Connection) DomainID() uint32 { return c.domainID }

// IsClosed reports whether Close has been called. Used by the hub to
// avoid re-queueing writes to a dead connection.
func (c *Connection) IsClosed() bool {
	c.closedMu.Lock()
	defer c.closedMu.Unlock()
	return c.closedFlag
}

// Close shuts the connection down exactly once. It closes the send
// channel (causing the writer goroutine to exit), invokes the
// closeHook, and (if a real socket was wired in) closes the gorilla
// socket with the given code/reason.
func (c *Connection) Close(code int, reason string) {
	c.closedMu.Lock()
	if c.closedFlag {
		c.closedMu.Unlock()
		return
	}
	c.closedFlag = true
	close(c.closed)
	c.closedMu.Unlock()

	if c.ws != nil {
		msg := websocket.FormatCloseMessage(code, reason)
		_ = c.ws.WriteControl(websocket.CloseMessage, msg, _deadlineSoon())
		_ = c.ws.Close()
	}
	if c.closeHook != nil {
		c.closeHook(c, reason)
	}
}

// Closed returns a channel that is closed when the connection is
// torn down. The hub's read loop and the writer goroutine select on
// it to abort cleanly.
func (c *Connection) Closed() <-chan struct{} { return c.closed }

// onAck is called by the connection's reader loop when the client
// sends a WsPushAck. It only enqueues the acknowledgement: the
// handler runs on the connection's ack worker so a slow database
// round trip cannot stall the socket reads.
func (c *Connection) onAck(messageID string) {
	if c.ackHandler == nil {
		return
	}
	select {
	case c.ackQueue <- messageID:
	default:
		if c.logger != nil {
			c.logger.Warn("realtime: ack queue full, dropping ack",
				slog.Uint64("device_id", c.deviceID))
		}
	}
}

// ackLoop processes the acknowledgements queued by onAck. It stops
// when the connection is closed, after draining whatever is still
// buffered so a client that acknowledges and disconnects in the same
// breath does not lose the ack.
func (c *Connection) ackLoop() {
	for {
		select {
		case <-c.closed:
			c.drainAcks()
			return
		case messageID := <-c.ackQueue:
			c.handleAck(messageID)
		}
	}
}

func (c *Connection) drainAcks() {
	for {
		select {
		case messageID := <-c.ackQueue:
			c.handleAck(messageID)
		default:
			return
		}
	}
}

func (c *Connection) handleAck(messageID string) {
	if c.ackHandler == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), ackTimeout)
	defer cancel()
	c.ackHandler(ctx, messageID)
}
