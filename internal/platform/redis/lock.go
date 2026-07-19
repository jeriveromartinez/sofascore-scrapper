package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var ErrLeaseLost = errors.New("redis lease lost")

type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, bool, error)
}

type Lease interface {
	Key() string
	Owner() string
	Renew(ctx context.Context, ttl time.Duration) error
	Release(ctx context.Context) error
}

type lease struct {
	key    string
	owner  string
	client *goredis.Client
}

func (l *lease) Key() string   { return l.key }
func (l *lease) Owner() string { return l.owner }

func (l *lease) Renew(ctx context.Context, ttl time.Duration) error {
	ok, err := renewScript.Run(ctx, l.client, []string{l.key}, l.owner, ttl.Milliseconds()).Bool()
	if err != nil {
		return fmt.Errorf("renew lease %s: %w", l.key, err)
	}
	if !ok {
		return ErrLeaseLost
	}
	return nil
}

func (l *lease) Release(ctx context.Context) error {
	ok, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.owner).Bool()
	if err != nil {
		return fmt.Errorf("release lease %s: %w", l.key, err)
	}
	if !ok {
		return ErrLeaseLost
	}
	return nil
}

type locker struct {
	client     *goredis.Client
	instanceID string
}

func NewLocker(client *goredis.Client) Locker {
	hostname, _ := os.Hostname()
	return &locker{
		client:     client,
		instanceID: hostname,
	}
}

func (l *locker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, bool, error) {
	if l.client == nil {
		return nil, false, ErrLeaseLost
	}

	owner, err := generateOwner(l.instanceID)
	if err != nil {
		return nil, false, fmt.Errorf("generate lease owner: %w", err)
	}

	ok, err := l.client.SetNX(ctx, key, owner, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("acquire lease %s: %w", key, err)
	}
	if !ok {
		return nil, false, nil
	}

	return &lease{key: key, owner: owner, client: l.client}, true, nil
}

func generateOwner(instanceID string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + ":" + instanceID, nil
}
