package middleware

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type NonceStore struct {
	client *redis.Client
}

func NewNonceStore(client *redis.Client) *NonceStore {
	return &NonceStore{client: client}
}

// MarkUsed atomically marks a nonce as used. Returns true if this is the first use
// (nonce is valid), false if already used (replay detected).
func (s *NonceStore) MarkUsed(ctx context.Context, jti string, ttl time.Duration) (bool, error) {
	if jti == "" {
		return false, nil
	}
	return s.client.SetNX(ctx, "nonce:"+jti, "1", ttl).Result()
}
