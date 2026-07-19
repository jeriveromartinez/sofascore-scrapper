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

type InvitationStore struct {
	redis  *goredis.Client
	now    func() time.Time
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

	hash := sha256.Sum256([]byte(token))
	hexHash := hex.EncodeToString(hash[:])
	key := invitationKeyPrefix + ":" + hexHash

	expiresAt := s.now().Add(ttl)
	if err := s.redis.Set(ctx, key, "1", ttl).Err(); err != nil {
		return "", time.Time{}, err
	}

	return token, expiresAt, nil
}

func (s *InvitationStore) Consume(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	hexHash := hex.EncodeToString(hash[:])
	key := invitationKeyPrefix + ":" + hexHash

	result, err := consumeLua.Run(ctx, s.redis, []string{key}).Result()
	if err != nil {
		return err
	}

	status, ok := result.(string)
	if !ok || status != "ok" {
		return ErrInvalidInvitation
	}

	return nil
}
