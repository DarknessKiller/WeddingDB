package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRefresh_UnknownTokenIsInvalid(t *testing.T) {
	svc, _, _ := newIntegrationAuthService(t, "secret")

	_, err := svc.Refresh(context.Background(), "no-such-token", "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("got %v, want ErrInvalidToken", err)
	}
}

// A Redis outage while minting the successor must not look like a dead token,
// or the client ends the session and drops its offline queue.
func TestRefresh_InfraFailureIsNotInvalidToken(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "secret")
	adminID := uuid.New()
	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, "valid-token")
	svc.redisClient = redis.NewClient(&redis.Options{Addr: "localhost:1", MaxRetries: -1})

	_, err := svc.Refresh(context.Background(), "valid-token", "")
	if err == nil {
		t.Fatal("expected refresh to fail without Redis")
	}
	if errors.Is(err, ErrInvalidToken) {
		t.Fatalf("infra failure classified as dead token: %v", err)
	}
}
