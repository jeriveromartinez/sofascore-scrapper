package realtime

import (
	"testing"
	"time"
)

// TestRegisterEvictingPreviousConnectionDoesNotDeadlock guards the
// self-deadlock in which Hub.Register closed the evicted connection
// while holding the hub mutex: Connection.Close runs the close hook
// synchronously and that hook calls back into the hub, so a routine
// device reconnect wedged every WebSocket delivery on the backend.
func TestRegisterEvictingPreviousConnectionDoesNotDeadlock(t *testing.T) {
	h := NewHub()

	old := newConnection(1, 0, 0, nil, nil, func(c *Connection, reason string) {
		h.UnregisterConn(1, c)
	}, nil)
	h.Register(1, old)

	replacement := newConnection(1, 0, 0, nil, nil, nil, nil)

	done := make(chan struct{})
	go func() {
		h.Register(1, replacement)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Register deadlocked while evicting the previous connection")
	}

	got, ok := h.Get(1)
	if !ok {
		t.Fatal("the replacement connection is no longer registered")
	}
	if got != replacement {
		t.Fatal("the evicted connection's close hook dropped the live replacement")
	}
}

// TestUnregisterConnKeepsReplacementRegistered asserts a late close of
// an evicted connection does not remove the connection that replaced
// it: both share the same device id.
func TestUnregisterConnKeepsReplacementRegistered(t *testing.T) {
	h := NewHub()

	old := newConnection(7, 0, 0, nil, nil, nil, nil)
	replacement := newConnection(7, 0, 0, nil, nil, nil, nil)

	h.Register(7, old)
	h.Register(7, replacement)

	h.UnregisterConn(7, old)

	got, ok := h.Get(7)
	if !ok {
		t.Fatal("the live connection was removed by the stale unregister")
	}
	if got != replacement {
		t.Fatal("the live connection was replaced by the stale unregister")
	}
}

// TestUnregisterConnRemovesItsOwnConnection asserts the normal
// disconnect path still frees the slot.
func TestUnregisterConnRemovesItsOwnConnection(t *testing.T) {
	h := NewHub()

	conn := newConnection(9, 0, 0, nil, nil, nil, nil)
	h.Register(9, conn)

	h.UnregisterConn(9, conn)

	if _, ok := h.Get(9); ok {
		t.Fatal("the connection was not removed")
	}
}
