package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestAuthRate_IPBurst25PerSecond(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	// Pre-seed the current per-second bucket just under the limit. Retry if
	// the wall-clock second ticks between seeding and the check.
	ok := false
	for attempt := 0; attempt < 3 && !ok; attempt++ {
		bucket := fmt.Sprintf("ratelimit:auth:ip:1.2.3.4:%d", time.Now().Unix())
		if err := mr.Set(bucket, "24"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		mr.SetTTL(bucket, time.Second)
		err25 := svc.CheckAuthRate(ctx, "1.2.3.4")
		err26 := svc.CheckAuthRate(ctx, "1.2.3.4")
		ok = err25 == nil && err26 != nil
	}
	if !ok {
		t.Fatal("25th request must pass and 26th in the same second must be limited")
	}
	if err := svc.CheckAuthRate(ctx, "5.6.7.8"); err != nil {
		t.Fatalf("other IPs must be unaffected: %v", err)
	}
}

func TestLoginAccountRate_Threshold(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	email := "victim@example.com"
	for i := 0; i < 25; i++ {
		if err := svc.CheckLoginAccountRate(ctx, email); err != nil {
			t.Fatalf("failure %d should still be under limit: %v", i+1, err)
		}
		svc.RecordLoginFailure(ctx, email)
	}
	if err := svc.CheckLoginAccountRate(ctx, email); err == nil {
		t.Fatal("expected account lockout after 25 failures")
	}
	if err := svc.CheckLoginAccountRate(ctx, "other@example.com"); err != nil {
		t.Fatalf("other account must stay unaffected: %v", err)
	}
}

func TestLoginAccountRate_WindowExpiry(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		svc.RecordLoginFailure(ctx, "w@x.y")
	}
	if err := svc.CheckLoginAccountRate(ctx, "w@x.y"); err == nil {
		t.Fatal("expected lockout before window expiry")
	}
	mr.FastForward(16 * time.Minute)
	if err := svc.CheckLoginAccountRate(ctx, "w@x.y"); err != nil {
		t.Fatalf("expected unlocked after window: %v", err)
	}
}

func TestAuthRate_FailsOpenWithoutRedis(t *testing.T) {
	bad := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	svc := &AuthService{secret: []byte("x"), redisClient: bad}
	if err := svc.CheckAuthRate(context.Background(), "1.2.3.4"); err != nil {
		t.Fatalf("expected fail-open without Redis, got %v", err)
	}
	if err := svc.CheckLoginAccountRate(context.Background(), "a@b.c"); err != nil {
		t.Fatalf("expected fail-open without Redis, got %v", err)
	}
	svc.RecordLoginFailure(context.Background(), "a@b.c") // must not panic
}
