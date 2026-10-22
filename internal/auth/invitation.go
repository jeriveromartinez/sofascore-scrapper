package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var ErrInvalidInvitation = errors.New("invalid invitation token")
var ErrInvitationExpired = errors.New("invitation token expired")

const invitationKeyPrefix = "auth:invitation"
const invitationReservedKeyPrefix = "auth:invitation-reserved"
const DefaultInvitationTTL = 24 * time.Hour
const MinInvitationTTL = 300 * time.Second
const MaxInvitationTTL = 604800 * time.Second

var consumeLua = goredis.NewScript(`
	local key = KEYS[1]
	if redis.call("EXISTS", key) == 0 then
		return redis.status_reply("not_found")
	end
	local deleted = redis.call("DEL", key)
	if deleted == 0 then
		return redis.status_reply("not_found")
	end
	return redis.status_reply("ok")
`)

// reserveLua moves the invitation to a reservation key. The token
// stays usable (it is not deleted) but no other registration can
// claim it while the reservation is held.
var reserveLua = goredis.NewScript(`
	local key = KEYS[1]
	local reserved = KEYS[2]
	if redis.call("EXISTS", key) == 0 then
		return redis.status_reply("not_found")
	end
	redis.call("RENAME", key, reserved)
	return redis.status_reply("ok")
`)

var releaseLua = goredis.NewScript(`
	local reserved = KEYS[1]
	local key = KEYS[2]
	if redis.call("EXISTS", reserved) == 0 then
		return redis.status_reply("not_found")
	end
	redis.call("RENAME", reserved, key)
	return redis.status_reply("ok")
`)

type InvitationStore struct {
	redis *goredis.Client
	now   func() time.Time
}

func NewInvitationStore(client *goredis.Client) *InvitationStore {
	return &InvitationStore{redis: client, now: time.Now}
}

func (s *InvitationStore) Create(ctx context.Context, ttl time.Duration) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	expiresAt := s.now().Add(ttl)
	if err := s.redis.Set(ctx, invitationKey(token), "1", ttl).Err(); err != nil {
		return "", time.Time{}, err
	}

	return token, expiresAt, nil
}

func (s *InvitationStore) Consume(ctx context.Context, token string) error {
	result, err := consumeLua.Run(ctx, s.redis, []string{invitationKey(token)}).Result()
	if err != nil {
		return err
	}

	status, ok := result.(string)
	if !ok || status != "ok" {
		return ErrInvalidInvitation
	}

	return nil
}

// Reserve claims an invitation for an in-flight registration without
// consuming it. The token is moved to a reservation key, so a second
// registration cannot use it while the first one is running, but the
// invitation is still usable if the registration fails and the caller
// releases it.
func (s *InvitationStore) Reserve(ctx context.Context, token string) error {
	result, err := reserveLua.Run(ctx, s.redis, []string{invitationKey(token), reservedInvitationKey(token)}).Result()
	if err != nil {
		return err
	}

	status, ok := result.(string)
	if !ok || status != "ok" {
		return ErrInvalidInvitation
	}

	return nil
}

// Commit consumes a reserved invitation. The reservation is dropped
// and the token cannot be used again.
func (s *InvitationStore) Commit(ctx context.Context, token string) error {
	return s.redis.Del(ctx, reservedInvitationKey(token)).Err()
}

// Release returns a reserved invitation to the pool of usable
// tokens. It is a no-op when the reservation is gone (already
// committed or expired).
func (s *InvitationStore) Release(ctx context.Context, token string) error {
	result, err := releaseLua.Run(ctx, s.redis, []string{reservedInvitationKey(token), invitationKey(token)}).Result()
	if err != nil {
		return err
	}

	status, ok := result.(string)
	if !ok || status != "ok" {
		return ErrInvalidInvitation
	}

	return nil
}

func invitationKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return invitationKeyPrefix + ":" + hex.EncodeToString(hash[:])
}

func reservedInvitationKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return invitationReservedKeyPrefix + ":" + hex.EncodeToString(hash[:])
}
