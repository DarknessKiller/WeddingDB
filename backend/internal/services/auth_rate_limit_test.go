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
		if err := svc.CheckLoginAccountRate(ctx, email, "1.2.3.4"); err != nil {
			t.Fatalf("failure %d should still be under limit: %v", i+1, err)
		}
		svc.RecordLoginFailure(ctx, email, "1.2.3.4")
	}
	if err := svc.CheckLoginAccountRate(ctx, email, "1.2.3.4"); err == nil {
		t.Fatal("expected account lockout after 25 failures")
	}
	if err := svc.CheckLoginAccountRate(ctx, "other@example.com", "1.2.3.4"); err != nil {
		t.Fatalf("other account must stay unaffected: %v", err)
	}
}

func TestLoginAccountRate_WindowExpiry(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		svc.RecordLoginFailure(ctx, "w@x.y", "1.2.3.4")
	}
	if err := svc.CheckLoginAccountRate(ctx, "w@x.y", "1.2.3.4"); err == nil {
		t.Fatal("expected lockout before window expiry")
	}
	mr.FastForward(16 * time.Minute)
	if err := svc.CheckLoginAccountRate(ctx, "w@x.y", "1.2.3.4"); err != nil {
		t.Fatalf("expected unlocked after window: %v", err)
	}
}

func TestAuthRate_FailsOpenWithoutRedis(t *testing.T) {
	bad := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	svc := &AuthService{secret: []byte("x"), redisClient: bad}
	if err := svc.CheckAuthRate(context.Background(), "1.2.3.4"); err != nil {
		t.Fatalf("expected fail-open without Redis, got %v", err)
	}
	if err := svc.CheckLoginAccountRate(context.Background(), "a@b.c", "1.2.3.4"); err != nil {
		t.Fatalf("expected fail-open without Redis, got %v", err)
	}
	svc.RecordLoginFailure(context.Background(), "a@b.c", "1.2.3.4") // must not panic
}

func TestRateLimit_KeyAlwaysHasTTL(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	svc.RecordLoginFailure(ctx, "V@x.y ", "9.9.9.9")
	if ttl := mr.TTL("ratelimit:login:acct:v@x.y:9.9.9.9"); ttl <= 0 {
		t.Fatalf("failure counter TTL = %v, want > 0", ttl)
	}
	if err := svc.CheckCredentialIPRate(ctx, "9.9.9.9"); err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL("ratelimit:cred:ip:9.9.9.9"); ttl <= 0 {
		t.Fatalf("ip counter TTL = %v, want > 0", ttl)
	}
}

func TestLoginLockout_ScopedToIP(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for range 25 {
		svc.RecordLoginFailure(ctx, "victim@x.y", "6.6.6.6")
	}
	if err := svc.CheckLoginAccountRate(ctx, "victim@x.y", "6.6.6.6"); err == nil {
		t.Fatal("attacker IP should be locked out")
	}
	if err := svc.CheckLoginAccountRate(ctx, "victim@x.y", "1.1.1.1"); err != nil {
		t.Fatalf("victim's own IP must not be locked out: %v", err)
	}
}

func TestCredentialIPRate_TenPerMinute(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := range 10 {
		if err := svc.CheckCredentialIPRate(ctx, "7.7.7.7"); err != nil {
			t.Fatalf("attempt %d limited early: %v", i+1, err)
		}
	}
	if err := svc.CheckCredentialIPRate(ctx, "7.7.7.7"); err == nil {
		t.Fatal("11th attempt in a minute must be limited")
	}
	mr.FastForward(61 * time.Second)
	if err := svc.CheckCredentialIPRate(ctx, "7.7.7.7"); err != nil {
		t.Fatalf("expected reset after window: %v", err)
	}
}
