package realtime

import (
	"sync"

	"github.com/gorilla/websocket"
)

// AckHandler is invoked when the client sends a WsPushAck back to the
// server. The implementation in internal/push uses this to flip a
// delivery_attempts row from SENT to DELIVERED.
type AckHandler func(messageID uint64)

// CloseHandler is invoked exactly once when the connection is being
// torn down (either by the server, by the client, or because of an
// error). It must be safe to call from any goroutine.
type CloseHandler func(reason string)

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
}

// newConnection constructs a Connection. ws may be nil in unit tests
// that exercise the hub without a real socket.
func newConnection(deviceID uint64, userID, domainID uint32, ws *websocket.Conn, ack AckHandler, closeHook CloseHandler) *Connection {
	return &Connection{
		deviceID:   deviceID,
		userID:     userID,
		domainID:   domainID,
		ws:         ws,
		send:       make(chan []byte, 16),
		closed:     make(chan struct{}),
		ackHandler: ack,
		closeHook:  closeHook,
	}
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
		c.closeHook(reason)
	}
}

// Closed returns a channel that is closed when the connection is
// torn down. The hub's read loop and the writer goroutine select on
// it to abort cleanly.
func (c *Connection) Closed() <-chan struct{} { return c.closed }

// onAck is called by the connection's reader loop when the client
// sends a WsPushAck. It dispatches to the ackHandler installed at
// construction time.
func (c *Connection) onAck(messageID uint64) {
	if c.ackHandler != nil {
		c.ackHandler(messageID)
	}
}
