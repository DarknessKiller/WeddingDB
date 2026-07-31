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

func (s *NonceStore) MarkUsed(ctx context.Context, jti string, ttl time.Duration) error {
	return s.client.Set(ctx, "nonce:"+jti, "1", ttl).Err()
}

func (s *NonceStore) IsUsed(ctx context.Context, jti string) bool {
	exists, err := s.client.Exists(ctx, "nonce:"+jti).Result()
	return err == nil && exists > 0
}

type TokenBlacklist struct {
	client *redis.Client
}

func NewTokenBlacklist(client *redis.Client) *TokenBlacklist {
	return &TokenBlacklist{client: client}
}

func (b *TokenBlacklist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	return b.client.Set(ctx, "bl:"+jti, "1", ttl).Err()
}

func (b *TokenBlacklist) IsRevoked(ctx context.Context, jti string) bool {
	exists, err := b.client.Exists(ctx, "bl:"+jti).Result()
	return err == nil && exists > 0
}
