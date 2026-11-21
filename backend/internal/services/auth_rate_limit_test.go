package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// failFrom spends n wrong-password attempts from ip, stepping past the
// per-IP minute window so only the 15-minute limits accumulate.
func failFrom(t *testing.T, svc *AuthService, mr interface{ FastForward(time.Duration) }, email, ip string, n int) {
	t.Helper()
	for i := range n {
		if i > 0 && i%loginIPLimit == 0 {
			mr.FastForward(61 * time.Second)
		}
		if err := svc.ReserveLoginAttempt(context.Background(), email, ip); err != nil {
			t.Fatalf("attempt %d refused early: %v", i+1, err)
		}
	}
}

// Parallel requests all reserve before bcrypt, so a burst against one
// account cannot get more guesses than the limit (a read-then-record check
// let all of them through).
func TestLoginAttempt_BurstCappedPerAccount(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := range loginAcctLimit {
		if err := svc.ReserveLoginAttempt(ctx, "a@x.y", "1.2.3.4"); err != nil {
			t.Fatalf("attempt %d refused early: %v", i+1, err)
		}
	}
	if err := svc.ReserveLoginAttempt(ctx, "a@x.y", "1.2.3.4"); err == nil {
		t.Fatal("26th in-flight attempt against one account must be refused")
	}
}

// The per-IP window counts wrong passwords only; once full, it blocks any
// account from that IP.
func TestLoginFailed_BlocksIP(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for range loginIPLimit {
		svc.LoginFailed(ctx, "1.2.3.4")
	}
	if err := svc.ReserveLoginAttempt(ctx, "b@x.y", "1.2.3.4"); err == nil {
		t.Fatal("attempt after 10 wrong passwords from one IP must be refused")
	}
	if err := svc.ReserveLoginAttempt(ctx, "b@x.y", "5.6.7.8"); err != nil {
		t.Fatalf("other IPs must be unaffected: %v", err)
	}
}

func TestLoginAttempt_SuccessGivesSlotBack(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := range 3 * loginIPLimit {
		if err := svc.ReserveLoginAttempt(ctx, "staff@x.y", "1.2.3.4"); err != nil {
			t.Fatalf("login %d refused: %v", i+1, err)
		}
		svc.ReleaseLoginAttempt(ctx, "staff@x.y", "1.2.3.4")
	}
}

// Staff behind one venue NAT sign in at once: successful logins never touch
// the per-IP window, so none of them is refused.
func TestLoginAttempt_ConcurrentSuccessesFromOneIP(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			email := fmt.Sprintf("staff%d@x.y", i)
			if err := svc.ReserveLoginAttempt(ctx, email, "1.2.3.4"); err != nil {
				errs <- err
				return
			}
			svc.ReleaseLoginAttempt(ctx, email, "1.2.3.4")
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent login from shared IP refused: %v", err)
	}
}

func TestLoginAccountRate_Threshold(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	failFrom(t, svc, mr, "victim@example.com", "1.2.3.4", loginAcctLimit)
	mr.FastForward(61 * time.Second)
	if err := svc.ReserveLoginAttempt(ctx, "victim@example.com", "1.2.3.4"); err == nil {
		t.Fatal("expected account lockout after 25 failures")
	}
	if err := svc.ReserveLoginAttempt(ctx, "other@example.com", "1.2.3.4"); err != nil {
		t.Fatalf("other account must stay unaffected: %v", err)
	}
}

func TestLoginAccountRate_WindowExpiry(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	failFrom(t, svc, mr, "w@x.y", "1.2.3.4", loginAcctLimit)
	mr.FastForward(16 * time.Minute)
	if err := svc.ReserveLoginAttempt(context.Background(), "w@x.y", "1.2.3.4"); err != nil {
		t.Fatalf("expected unlocked after window: %v", err)
	}
}

func TestLoginEmailRate_CapsAcrossIPs(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	for i := range loginEmailLimit {
		if err := svc.ReserveLoginAttempt(ctx, "victim@x.y", fmt.Sprintf("10.0.0.%d", i)); err != nil {
			t.Fatalf("attempt %d refused early: %v", i+1, err)
		}
	}
	if err := svc.ReserveLoginAttempt(ctx, "victim@x.y", "10.0.1.1"); err == nil {
		t.Fatal("guesses spread over many IPs must hit the per-email cap")
	}
}

func TestLoginRate_FailsClosedWithoutRedis(t *testing.T) {
	bad := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	svc := &AuthService{secret: []byte("x"), redisClient: bad}
	if err := svc.CheckAuthRate(context.Background(), "1.2.3.4"); err != nil {
		t.Fatalf("request-rate limit must fail open without Redis, got %v", err)
	}
	// Password-guess limits fail closed: outage must not allow unlimited guesses.
	if err := svc.ReserveLoginAttempt(context.Background(), "a@b.c", "1.2.3.4"); !errors.Is(err, ErrRateLimitUnavailable) {
		t.Fatalf("expected fail-closed without Redis, got %v", err)
	}
	svc.ReleaseLoginAttempt(context.Background(), "a@b.c", "1.2.3.4") // must not panic
}

func TestRateLimit_KeyAlwaysHasTTL(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	// Seed keys with no TTL. Incr-then-Expire-only-if-n==1 (pre-fix
	// implementation) would leave them permanent; only ExpireNX repairs them.
	for _, k := range []string{"ratelimit:login:acct:v@x.y:9.9.9.9", "ratelimit:login:email:v@x.y", "ratelimit:cred:ip:9.9.9.9"} {
		if err := mr.Set(k, "0"); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
	if err := svc.ReserveLoginAttempt(ctx, "V@x.y ", "9.9.9.9"); err != nil {
		t.Fatal(err)
	}
	svc.LoginFailed(ctx, "9.9.9.9")
	for _, k := range []string{"ratelimit:login:acct:v@x.y:9.9.9.9", "ratelimit:login:email:v@x.y", "ratelimit:cred:ip:9.9.9.9"} {
		if ttl := mr.TTL(k); ttl <= 0 {
			t.Fatalf("%s TTL = %v, want > 0", k, ttl)
		}
	}
}

// Releasing after the window expired must not create a TTL-less key at -1,
// which would hand out an extra guess forever.
func TestReleaseLoginAttempt_ExpiredWindowStaysGone(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	ctx := context.Background()
	if err := svc.ReserveLoginAttempt(ctx, "r@x.y", "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(16 * time.Minute)
	svc.ReleaseLoginAttempt(ctx, "r@x.y", "8.8.8.8")
	if mr.Exists("ratelimit:login:email:r@x.y") {
		t.Fatal("release recreated an expired window")
	}
}

func TestLoginLockout_ScopedToIP(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret")
	failFrom(t, svc, mr, "victim@x.y", "6.6.6.6", loginAcctLimit)
	mr.FastForward(61 * time.Second)
	if err := svc.ReserveLoginAttempt(context.Background(), "victim@x.y", "6.6.6.6"); err == nil {
		t.Fatal("attacker IP should be locked out")
	}
	if err := svc.ReserveLoginAttempt(context.Background(), "victim@x.y", "1.1.1.1"); err != nil {
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
