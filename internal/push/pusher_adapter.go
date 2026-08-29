package push

import (
	"context"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
)

// realtimeSubscriberPusher adapts realtime.Subscriber to the
// Pusher interface the push.Service consumes. The adapter is
// trivial (the only method is PublishPush) but kept as its own
// type so the wire-up in internal/app is greppable and so a
// future swap (e.g. to a different broker) is a one-file change.
type realtimeSubscriberPusher struct {
	sub *realtime.Subscriber
}

// NewRealtimeSubscriberPusher returns a Pusher that broadcasts
// every WsPush through the realtime Redis subscriber.
func NewRealtimeSubscriberPusher(sub *realtime.Subscriber) Pusher {
	return &realtimeSubscriberPusher{sub: sub}
}

// PublishPush encodes the frame and ships it on the realtime
// pubsub channel. Sibling backend instances receive the broadcast
// and their local hubs decide whether the device is local.
func (r *realtimeSubscriberPusher) PublishPush(ctx context.Context, deviceID uint64, push *pb.WsPush) error {
	return r.sub.Publish(ctx, deviceID, push)
}
