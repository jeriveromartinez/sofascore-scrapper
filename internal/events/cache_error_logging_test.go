package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingCache struct {
	getErr error
	setErr error
}

func (c *failingCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, c.getErr
}

func (c *failingCache) Set(context.Context, string, []byte, time.Duration) error {
	return c.setErr
}

func TestServiceLogsCacheGetErrorAndFallsBackToDatabase(t *testing.T) {
	cache := &failingCache{getErr: errors.New("redis unreachable")}
	svc, _ := newCachedServiceWithCache(t, cache)
	records := captureSlog(t)

	if _, err := svc.GetCurrentAndUpcoming(context.Background(), 1, 6); err != nil {
		t.Fatalf("GetCurrentAndUpcoming: %v", err)
	}

	rec := records.find(t, "cache lookup failed")
	assertStructuredWarn(t, rec, "err")
}

func TestServiceLogsCacheSetError(t *testing.T) {
	cache := &failingCache{setErr: errors.New("redis write refused")}
	svc, _ := newCachedServiceWithCache(t, cache)
	records := captureSlog(t)

	if _, err := svc.GetCurrentAndUpcoming(context.Background(), 1, 6); err != nil {
		t.Fatalf("GetCurrentAndUpcoming: %v", err)
	}

	rec := records.find(t, "cache write failed")
	assertStructuredWarn(t, rec, "err")
}
