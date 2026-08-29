package realtime

import (
	"sync"
	"testing"
	"time"
)

// TestHub_RegisterLookupUnregister covers the basic CRUD: a registered
// device is findable, an unregistered one is not, and Count reflects
// the current population.
func TestHub_RegisterLookupUnregister(t *testing.T) {
	h := NewHub()

	c1 := newTestConnection(t, 1, 1, 1)
	c2 := newTestConnection(t, 2, 1, 1)
	h.Register(1, c1)
	h.Register(2, c2)

	if got, ok := h.Get(1); !ok || got != c1 {
		t.Errorf("Get(1) = (%v, %v), want (%v, true)", got, ok, c1)
	}
	if got, ok := h.Get(2); !ok || got != c2 {
		t.Errorf("Get(2) = (%v, %v), want (%v, true)", got, ok, c2)
	}
	if h.Count() != 2 {
		t.Errorf("Count = %d, want 2", h.Count())
	}

	h.Unregister(1)
	if _, ok := h.Get(1); ok {
		t.Error("Get(1) after Unregister: ok=true, want false")
	}
	if h.Count() != 1 {
		t.Errorf("Count after Unregister = %d, want 1", h.Count())
	}

	// Close both to release the WS goroutines.
	c2.Close(1000, "test done")
}

// TestHub_DeliverToLocal verifies that DeliverToLocal routes a frame
// to exactly the connection whose device_id matches.
func TestHub_DeliverToLocal(t *testing.T) {
	h := NewHub()

	c1 := newTestConnection(t, 1, 1, 1)
	c2 := newTestConnection(t, 2, 1, 1)
	defer c2.Close(1000, "test done")
	h.Register(1, c1)
	h.Register(2, c2)

	// Push to device 1 only.
	if err := h.DeliverToLocal(1, &testPush{ID: 99, MessageID: 7}); err != nil {
		t.Fatalf("DeliverToLocal(1): %v", err)
	}

	// c1 must receive the frame.
	select {
	case got := <-c1.send:
		if len(got) == 0 {
			t.Fatal("c1 got empty frame")
		}
	case <-time.After(time.Second):
		t.Fatal("c1 did not receive the frame within 1s")
	}

	// c2 must NOT receive anything.
	select {
	case got := <-c2.send:
		t.Fatalf("c2 received an unexpected frame: %d bytes", len(got))
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// TestHub_DeliverToLocal_UnknownDeviceIsNoop verifies that pushing
// to a device that has no local connection (e.g. it's connected to
// a different backend instance) returns the sentinel
// ErrDeviceNotConnected, which the caller uses to record
// delivery_attempts.state = failed / failure_reason = device_offline.
func TestHub_DeliverToLocal_UnknownDeviceIsNoop(t *testing.T) {
	h := NewHub()
	err := h.DeliverToLocal(404, &testPush{ID: 1, MessageID: 1})
	if err != ErrDeviceNotConnected {
		t.Errorf("err = %v, want ErrDeviceNotConnected", err)
	}
}

// TestHub_RegisterTwiceForSameDeviceEvictsOldConnection guards against
// the race where a client reconnects (e.g. flaky network) before the
// old close handler runs: the new connection must win, and the old
// must be closed to release its goroutines.
func TestHub_RegisterTwiceForSameDeviceEvictsOldConnection(t *testing.T) {
	h := NewHub()

	c1 := newTestConnection(t, 1, 1, 1)
	c2 := newTestConnection(t, 1, 1, 1) // same device_id

	h.Register(1, c1)
	h.Register(1, c2) // evicts c1

	if got, _ := h.Get(1); got != c2 {
		t.Errorf("Get(1) = %v, want c2", got)
	}

	// c1 must be closed (writes should fail or channel be drained).
	c1.closedMu.Lock()
	closed := c1.closedFlag
	c1.closedMu.Unlock()
	if !closed {
		t.Error("c1 was not closed after eviction")
	}

	c2.Close(1000, "test done")
}

// TestHub_ConcurrentRegisterAndUnregister is a smoke test: the hub
// must remain consistent under concurrent register/unregister
// traffic (e.g. a deployment rolling out with hundreds of devices
// reconnecting at once).
func TestHub_ConcurrentRegisterAndUnregister(t *testing.T) {
	h := NewHub()
	const N = 50
	conns := make([]*Connection, N)
	for i := 0; i < N; i++ {
		conns[i] = newTestConnection(t, uint64(i+1), 1, 1)
	}

	var wg sync.WaitGroup
	wg.Add(2 * N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			h.Register(uint64(i+1), conns[i])
		}()
		go func() {
			defer wg.Done()
			h.Unregister(uint64(i + 1))
		}()
	}
	wg.Wait()

	// All writes from this point must not race (the goal of the
	// test). The exact final count is timing-dependent, so we only
	// assert that the hub is still usable.
	_ = h.Count()
	for _, c := range conns {
		_ = c
	}
}
