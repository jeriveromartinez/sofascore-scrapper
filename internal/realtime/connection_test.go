package realtime

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConnection_OnAckDoesNotBlockReader asserts that the reader loop
// is never blocked by the ack handler. The handler performs a database
// round trip, and running it inline stalls every subsequent frame the
// client sends.
func TestConnection_OnAckDoesNotBlockReader(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	conn := newConnection(1, 1, 1, nil, func(ctx context.Context, messageID string) {
		close(entered)
		<-release
	}, nil, nil)
	defer conn.Close(1000, "test done")

	returned := make(chan struct{})
	go func() {
		conn.onAck("message-1")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("onAck blocked the reader on the ack handler")
	}

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("the ack handler was never invoked")
	}
	close(release)
}

// TestConnection_AckSurvivesConnectionClose asserts that an
// acknowledgement received right before the client disconnects is
// still persisted.
func TestConnection_AckSurvivesConnectionClose(t *testing.T) {
	var mu sync.Mutex
	received := make([]string, 0, 1)
	entered := make(chan struct{})
	conn := newConnection(1, 1, 1, nil, func(ctx context.Context, messageID string) {
		mu.Lock()
		received = append(received, messageID)
		mu.Unlock()
		close(entered)
	}, nil, nil)

	conn.onAck("message-1")
	conn.Close(1000, "client gone")

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the queued ack was dropped when the connection closed")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 || received[0] != "message-1" {
		t.Fatalf("received = %v, want [message-1]", received)
	}
}

// TestConnection_AckHandlerContextOutlivesConnection asserts that the
// ack handler gets a context that is not cancelled by the socket going
// away: the delivery row must still be updated.
func TestConnection_AckHandlerContextOutlivesConnection(t *testing.T) {
	entered := make(chan struct{})
	ctxErr := make(chan error, 1)
	conn := newConnection(1, 1, 1, nil, func(ctx context.Context, messageID string) {
		close(entered)
		ctxErr <- ctx.Err()
	}, nil, nil)

	conn.onAck("message-1")
	conn.Close(1000, "client gone")

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the queued ack was dropped when the connection closed")
	}

	if err := <-ctxErr; err != nil {
		t.Fatalf("the ack context was cancelled with the connection: %v", err)
	}
}
