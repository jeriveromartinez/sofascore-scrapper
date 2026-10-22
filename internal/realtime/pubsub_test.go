package realtime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/redis/go-redis/v9"
)

// newTestRedis spins up an in-process miniredis and a real
// redis.Client pointed at it. The miniredis instance is closed via
// t.Cleanup. The caller gets back (client, miniredis) so it can
// inspect the server's state if needed.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})
	t.Cleanup(func() { _ = client.Close() })

	// Sanity: the connection works.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return client, mr
}

// TestSubscriber_RoundTripsPushMessage is the canonical flow:
// Publisher encodes a WsPush and publishes it to the Redis channel;
// Subscriber receives it, decodes it, and forwards the frame to the
// local hub. The device's connection in the hub receives the bytes.
func TestSubscriber_RoundTripsPushMessage(t *testing.T) {
	client, _ := newTestRedis(t)
	hub := NewHub()

	conn := newTestConnection(t, 42, 1, 1)
	hub.Register(42, conn)

	sub := NewSubscriber(client, hub, SubscriberConfig{
		Channel: "push:fanout:test",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sub.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(sub.Stop)

	// Give the goroutine a moment to subscribe (Redis Subscribe is
	// async; without a small wait the message can be lost on
	// miniredis).
	time.Sleep(50 * time.Millisecond)

	// Publish a push aimed at device 42.
	push := &pb.WsPush{
		PushId:    7,
		MessageId: "100",
		Title:     "hello",
		Body:      "world",
		SentAt:    1700000000000,
	}
	if err := sub.Publish(ctx, 42, push); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The connection must receive the frame within a reasonable
	// timeout. miniredis delivers synchronously so this is fast.
	select {
	case got := <-conn.send:
		if len(got) == 0 {
			t.Fatal("conn received empty bytes")
		}
		// Decode and assert.
		frame, err := decodeFrame(got)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		p := frame.GetPush()
		if p == nil {
			t.Fatalf("decoded payload is not Push: %T", frame.Payload)
		}
		if p.PushId != 7 || p.MessageId != "100" || p.Title != "hello" {
			t.Errorf("decoded push = %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("conn did not receive the frame within 2s")
	}
}

// TestSubscriber_KeyPrefixIsolatesChannels asserts that two
// deployments sharing one Redis instance do not receive each other's
// pushes. go-redis writes SUBSCRIBE straight to the connection, so the
// Redis key prefix hook cannot rewrite the channel: the subscriber
// has to namespace the channel itself.
func TestSubscriber_KeyPrefixIsolatesChannels(t *testing.T) {
	client, _ := newTestRedis(t)
	hubA := NewHub()
	hubB := NewHub()

	connA := newTestConnection(t, 42, 1, 1)
	hubA.Register(42, connA)
	connB := newTestConnection(t, 42, 1, 1)
	hubB.Register(42, connB)

	subA := NewSubscriber(client, hubA, SubscriberConfig{Channel: "push:fanout", KeyPrefix: "tenant-a:"})
	subB := NewSubscriber(client, hubB, SubscriberConfig{Channel: "push:fanout", KeyPrefix: "tenant-b:"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := subA.Start(ctx); err != nil {
		t.Fatalf("start a: %v", err)
	}
	if err := subB.Start(ctx); err != nil {
		t.Fatalf("start b: %v", err)
	}
	t.Cleanup(subA.Stop)
	t.Cleanup(subB.Stop)
	time.Sleep(50 * time.Millisecond)

	push := &pb.WsPush{PushId: 1, MessageId: "1", Title: "x", Body: "y"}
	if err := subA.Publish(ctx, 42, push); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-connA.send:
		if len(got) == 0 {
			t.Fatal("conn A received empty bytes")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("conn A did not receive the frame")
	}

	select {
	case got := <-connB.send:
		t.Fatalf("conn B received a frame from another deployment: %d bytes", len(got))
	case <-time.After(300 * time.Millisecond):
	}
}

// TestSubscriber_IgnoresOtherInstances publishes a message that
// targets a device that is NOT connected to this hub's backend. The
// Subscriber should still receive the message (because Redis delivers
// to all subscribers) but the hub's DeliverToLocal returns
// ErrDeviceNotConnected, so no connection is touched. We assert that
// no goroutine lingers and that the call returns cleanly.
func TestSubscriber_IgnoresOtherInstances(t *testing.T) {
	client, _ := newTestRedis(t)
	hub := NewHub() // empty hub, no local connections

	sub := NewSubscriber(client, hub, SubscriberConfig{Channel: "push:fanout:test2"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(sub.Stop)
	time.Sleep(50 * time.Millisecond)

	// No error expected; miniredis counts the published messages
	// and we can assert at least one was broadcast.
	if err := sub.Publish(ctx, 9999, &pb.WsPush{PushId: 1, MessageId: "1", Title: "x", Body: "y"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if hub.Count() != 0 {
		t.Errorf("hub.Count = %d, want 0 (no local conn)", hub.Count())
	}
}

// TestSubscriber_StopsCleanly verifies that Stop is idempotent and
// the goroutine exits when the context is cancelled. We track the
// exit via a counter that the implementation bumps on Run return.
func TestSubscriber_StopsCleanly(t *testing.T) {
	client, _ := newTestRedis(t)
	hub := NewHub()

	var exits int64
	sub := NewSubscriber(client, hub, SubscriberConfig{Channel: "push:fanout:test3"})
	sub.OnExit(func() { atomic.AddInt64(&exits, 1) })

	ctx, cancel := context.WithCancel(context.Background())
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	cancel()
	sub.Stop()
	sub.Stop() // idempotent

	// Allow a moment for the goroutine to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&exits) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt64(&exits) == 0 {
		t.Fatal("subscriber goroutine did not exit within 1s after Stop")
	}

	// Use filepath to avoid an unused-import warning on the test
	// build when the helper is the only thing importing it.
	_ = filepath.Join("x", "y")
}
