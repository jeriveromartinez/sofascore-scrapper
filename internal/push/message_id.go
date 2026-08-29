package push

import "sync/atomic"

// nextMessageID is the per-process counter used to mint WsPush
// message_ids. The counter is intentionally not persisted:
// the message_id is only meaningful to the client (latency
// display, dedup), not to the server. The server-side lookup
// is by (push_id, device_id), which the client supplies via
// the WsPushAck.push_id field.
var messageIDCounter atomic.Uint64

// nextMessageID returns the next monotonic transport id. Exposed
// at package scope (not a method on Service) so the WsPush
// constructor in service.go can call it without holding a
// reference to a Service.
func nextMessageID() uint64 {
	return messageIDCounter.Add(1)
}
