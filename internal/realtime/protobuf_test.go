package realtime

import (
	"testing"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// TestEncodeDecodeHello pins the round-trip of a WsHello frame: the
// server emits this on connect so the client can correlate the local
// device_id with the server-side one.
func TestEncodeDecodeHello(t *testing.T) {
	original := &pb.WsFrame{Payload: &pb.WsFrame_Hello{Hello: &pb.WsHello{
		DeviceId:   8123,
		ServerTime: 1700000000,
	}}}

	raw, err := encodeFrame(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("encode produced empty bytes")
	}

	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	hello := got.GetHello()
	if hello == nil {
		t.Fatalf("decoded payload is not Hello: %T", got.Payload)
	}
	if hello.DeviceId != 8123 {
		t.Errorf("DeviceId = %d, want 8123", hello.DeviceId)
	}
	if hello.ServerTime != 1700000000 {
		t.Errorf("ServerTime = %d, want 1700000000", hello.ServerTime)
	}
}

// TestEncodeDecodePush pins the round-trip of a WsPush, including the
// map<string,string> data field which is the trickiest to round-trip
// over the wire.
func TestEncodeDecodePush(t *testing.T) {
	original := &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
		PushId:     42,
		MessageId:  "99",
		Category:   pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
		Title:      "Goal!",
		Body:       "Real Madrid 1 - 0 Barcelona",
		ImageUrl:   "https://cdn.example.com/img.png",
		Priority:   pb.PushPriority_PUSH_PRIORITY_HIGH,
		TtlSeconds: 300,
		Data:       map[string]string{"match_id": "123", "league": "laliga"},
		SentAt:     1700000001000,
	}}}

	raw, err := encodeFrame(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	push := got.GetPush()
	if push == nil {
		t.Fatalf("decoded payload is not Push: %T", got.Payload)
	}
	if push.PushId != 42 || push.MessageId != "99" {
		t.Errorf("ids = (%d, %q), want (42, \"99\")", push.PushId, push.MessageId)
	}
	if push.Category != pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT {
		t.Errorf("Category = %v", push.Category)
	}
	if push.Title != "Goal!" || push.Body != "Real Madrid 1 - 0 Barcelona" {
		t.Errorf("title/body mismatch: %q / %q", push.Title, push.Body)
	}
	if push.Priority != pb.PushPriority_PUSH_PRIORITY_HIGH {
		t.Errorf("Priority = %v", push.Priority)
	}
	if push.TtlSeconds != 300 {
		t.Errorf("TtlSeconds = %d", push.TtlSeconds)
	}
	if push.Data["match_id"] != "123" || push.Data["league"] != "laliga" {
		t.Errorf("Data = %v", push.Data)
	}
}

// TestEncodeDecodePushAck is the client's response to a WsPush. The
// server uses this to flip delivery_attempts to DELIVERED.
func TestEncodeDecodePushAck(t *testing.T) {
	original := &pb.WsFrame{Payload: &pb.WsFrame_PushAck{PushAck: &pb.WsPushAck{
		MessageId: "99",
		AckedAt:   1700000001500,
	}}}
	raw, err := encodeFrame(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ack := got.GetPushAck()
	if ack == nil {
		t.Fatalf("decoded payload is not PushAck: %T", got.Payload)
	}
	if ack.MessageId != "99" {
		t.Errorf("MessageId = %q, want \"99\"", ack.MessageId)
	}
	if ack.AckedAt != 1700000001500 {
		t.Errorf("AckedAt = %d", ack.AckedAt)
	}
}

// TestEncodeDecodePingPong pins the keepalive frames. They are
// symmetric (server sends ping, client replies pong), so the same
// codec path is exercised twice.
func TestEncodeDecodePingPong(t *testing.T) {
	ping := &pb.WsFrame{Payload: &pb.WsFrame_Ping{Ping: &pb.WsPing{SentAt: 12345}}}
	raw, err := encodeFrame(ping)
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode ping: %v", err)
	}
	if got.GetPing() == nil || got.GetPing().SentAt != 12345 {
		t.Errorf("ping round-trip failed: %+v", got.GetPing())
	}

	pong := &pb.WsFrame{Payload: &pb.WsFrame_Pong{Pong: &pb.WsPong{SentAt: 67890}}}
	raw, err = encodeFrame(pong)
	if err != nil {
		t.Fatalf("encode pong: %v", err)
	}
	got, err = decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode pong: %v", err)
	}
	if got.GetPong() == nil || got.GetPong().SentAt != 67890 {
		t.Errorf("pong round-trip failed: %+v", got.GetPong())
	}
}

// TestEncodeDecodeError pins the WsError codec, which is the server's
// last message before closing the socket (see the close codes table
// in the spec).
func TestEncodeDecodeError(t *testing.T) {
	original := &pb.WsFrame{Payload: &pb.WsFrame_Error{Error: &pb.WsError{
		Code:    "invalid_token",
		Message: "device token not found",
	}}}
	raw, err := encodeFrame(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	e := got.GetError()
	if e == nil {
		t.Fatalf("decoded payload is not Error: %T", got.Payload)
	}
	if e.Code != "invalid_token" || e.Message != "device token not found" {
		t.Errorf("Error = %+v", e)
	}
}

// TestDecodeRejectsEmpty verifies the codec refuses empty input. The
// upgrade handler treats this as a fatal close and never trusts the
// client again on the same connection.
func TestDecodeRejectsEmpty(t *testing.T) {
	if _, err := decodeFrame(nil); err == nil {
		t.Fatal("expected error for nil bytes")
	}
	if _, err := decodeFrame([]byte{}); err == nil {
		t.Fatal("expected error for empty bytes")
	}
}
