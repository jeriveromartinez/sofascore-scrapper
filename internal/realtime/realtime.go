package realtime

import "time"

// _deadlineSoon returns a near-future time.Time used as the
// WriteControl deadline when closing a socket. Short enough to not
// block a shutdown for long, long enough to flush the close frame.
func _deadlineSoon() time.Time {
	return time.Now().Add(time.Second)
}
