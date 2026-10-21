package services

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLoginRateLimit_AccountThreshold(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	email := "victim@example.com"
	for i := 0; i < 5; i++ {
		if err := svc.CheckLoginRate(ctx, "1.2.3.4", email); err != nil {
			t.Fatalf("failure %d should still be under limit: %v", i+1, err)
		}
		svc.RecordLoginFailure(ctx, "1.2.3.4", email)
	}
	if err := svc.CheckLoginRate(ctx, "9.9.9.9", email); err == nil {
		t.Fatal("expected account lockout from any IP after 5 failures")
	}
	if err := svc.CheckLoginRate(ctx, "1.2.3.4", "other@example.com"); err != nil {
		t.Fatalf("other account must stay unaffected: %v", err)
	}
}

func TestLoginRateLimit_IPThreshold(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		svc.RecordLoginFailure(ctx, "203.0.113.9", "a@example.com")
	}
	if err := svc.CheckLoginRate(ctx, "203.0.113.9", "never-tried@example.com"); err == nil {
		t.Fatal("expected IP lockout after 10 failures")
	}
}

func TestLoginRateLimit_WindowExpiry(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		svc.RecordLoginFailure(ctx, "1.1.1.1", "w@x.y")
	}
	if err := svc.CheckLoginRate(ctx, "1.1.1.1", "w@x.y"); err == nil {
		t.Fatal("expected lockout before window expiry")
	}
	mr.FastForward(16 * time.Minute)
	if err := svc.CheckLoginRate(ctx, "1.1.1.1", "w@x.y"); err != nil {
		t.Fatalf("expected unlocked after window: %v", err)
	}
}

func TestLoginRateLimit_FailsOpenWithoutRedis(t *testing.T) {
	bad := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	svc := &AuthService{secret: []byte("x"), redisClient: bad}
	if err := svc.CheckLoginRate(context.Background(), "1.2.3.4", "a@b.c"); err != nil {
		t.Fatalf("expected fail-open without Redis, got %v", err)
	}
	svc.RecordLoginFailure(context.Background(), "1.2.3.4", "a@b.c") // must not panic
}
