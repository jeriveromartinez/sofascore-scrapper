package push

import "github.com/google/uuid"

// nextMessageID returns a globally unique UUID v4 string used as the
// transport message_id. The id is persisted in delivery_attempts.message_id
// (UNIQUE) and echoed by the client in WsPushAck.
//
// We use UUID v4 instead of a per-process counter because the column is
// UNIQUE across the whole table — a per-process atomic.Uint64 would
// collide across multiple backend instances behind the same load balancer.
func nextMessageID() string {
	return uuid.NewString()
}
