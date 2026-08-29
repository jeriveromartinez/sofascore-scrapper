// Package realtime is the WebSocket transport for the push-notifications
// feature. It is intentionally domain-agnostic: it speaks proto frames
// over a WebSocket, registers/unregisters connections by device_id, and
// dispatches frames received from Redis pub/sub. All business rules
// (audience filter, delivery state, metrics) live in internal/push.
package realtime

import (
	"errors"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

// ErrEmptyFrame is returned by decodeFrame when the client sends an
// empty payload. The handler should close the connection with a
// protocol error in that case.
var ErrEmptyFrame = errors.New("realtime: empty frame")

// encodeFrame serializes a WsFrame to its wire bytes. Frames are
// binary (application/x-protobuf) on the WebSocket; the proto.Marshal
// output is exactly what the upgrade handler writes to the socket.
func encodeFrame(f *pb.WsFrame) ([]byte, error) {
	return proto.Marshal(f)
}

// decodeFrame parses a WsFrame from wire bytes. Empty input returns
// ErrEmptyFrame; malformed proto returns whatever proto.Unmarshal
// reports (caller decides whether to close).
func decodeFrame(b []byte) (*pb.WsFrame, error) {
	if len(b) == 0 {
		return nil, ErrEmptyFrame
	}
	var f pb.WsFrame
	if err := proto.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
