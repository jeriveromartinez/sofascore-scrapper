package realtime

import (
	"testing"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// testPush implements the FramePayload interface for tests so we can
// pass a minimal struct without depending on the proto enum for
// delivery tests. The hub only cares about the encoded bytes.
type testPush struct {
	ID        uint64
	MessageID string
}

// encode implements FramePayload: returns the wire bytes the hub will
// hand to the connection's writer.
func (p *testPush) encode() ([]byte, error) {
	frame := &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
		PushId:    p.ID,
		MessageId: p.MessageID,
		Title:     "test",
		Body:      "body",
	}}}
	return encodeFrame(frame)
}

// newTestConnection builds a Connection wired to an in-memory pipe
// rather than a real gorilla socket. The two halves talk through a
// net.Pipe-equivalent (we use a custom channel pair to avoid pulling
// in net just for tests).
//
// The returned Connection is safe to use in tests:
//   - send is a buffered channel of wire bytes
//   - closedFlag is observable to verify the hub evicted the conn
//   - Close() is idempotent
func newTestConnection(t *testing.T, deviceID uint64, userID, domainID uint32) *Connection {
	t.Helper()

	c := &Connection{
		deviceID:   deviceID,
		userID:     userID,
		domainID:   domainID,
		send:       make(chan []byte, 16),
		closed:     make(chan struct{}),
		ackHandler: func(string) {},
		ws:         nil, // tests don't write through gorilla
		logger:     nil,
	}
	c.closedMu.Lock()
	c.closedFlag = false
	c.closedMu.Unlock()
	return c
}
