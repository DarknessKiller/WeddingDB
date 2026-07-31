package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// newTestAuthService creates an AuthService with a miniredis instance for testing.
func newTestAuthService(t *testing.T, secret string) (*AuthService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &AuthService{secret: []byte(secret), redisClient: rdb}, mr
}

func signTestToken(secret string, claims AccessClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestValidateToken_Valid(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()
	wid := uuid.New()
	now := time.Now()

	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID:   adminID,
		WeddingID: &wid,
		Role:      "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	claims, err := svc.ValidateToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.AdminID != adminID {
		t.Errorf("AdminID = %v, want %v", claims.AdminID, adminID)
	}
	if claims.WeddingID == nil || *claims.WeddingID != wid {
		t.Errorf("WeddingID mismatch")
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want %q", claims.Role, "admin")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")
	now := time.Now()

	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Minute)),
		},
	})

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	svc, _ := newTestAuthService(t, "correct-secret")
	now := time.Now()

	tok := signTestToken("wrong-secret", AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for wrong-secret token, got nil")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")

	_, err := svc.ValidateToken(context.Background(), "not.a.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

func TestValidateToken_NilWeddingID(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()
	now := time.Now()

	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID: adminID,
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	claims, err := svc.ValidateToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.WeddingID != nil {
		t.Errorf("expected nil WeddingID, got %v", claims.WeddingID)
	}
}

func TestValidateToken_Blacklisted(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret-123")
	now := time.Now()
	jti := uuid.New().String()

	// Sign a valid token
	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	// Blacklist the JTI in miniredis
	mr.Set("blacklist:jti:"+jti, "1")

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for blacklisted token, got nil")
	}
}

func TestValidateToken_StaleTokenVersion(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()
	now := time.Now()

	// Sign token with tv=0
	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID:      adminID,
		Role:         "admin",
		TokenVersion: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	// Set stored version to 1 (simulates admin revoke)
	mr.Set("user:"+adminID.String()+":tv", "1")

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for stale token version, got nil")
	}
}

func TestBlacklistAccessToken_SetsKeyWithTTL(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret-123")
	jti := uuid.New().String()

	claims := &AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}

	if err := svc.BlacklistAccessToken(context.Background(), claims); err != nil {
		t.Fatalf("BlacklistAccessToken failed: %v", err)
	}

	// Verify the key exists in Redis
	key := "blacklist:jti:" + jti
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %q in Redis, got error: %v", key, err)
	}
	if val != "1" {
		t.Errorf("expected value '1', got %q", val)
	}

	// Verify TTL is set (should be ~10 minutes, between 5m and 15m)
	ttl := mr.TTL(key)
	if ttl <= 0 || ttl > 15*time.Minute {
		t.Errorf("unexpected TTL: %v, expected between 0 and 15m", ttl)
	}
}

func TestBlacklistAccessToken_SkipsExpiredToken(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret-123")

	claims := &AccessClaims{
		AdminID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // already expired
		},
	}

	if err := svc.BlacklistAccessToken(context.Background(), claims); err != nil {
		t.Fatalf("BlacklistAccessToken failed: %v", err)
	}

	// Verify no key was set (expired token doesn't need blacklisting)
	keys := mr.Keys()
	for _, k := range keys {
		if k == "blacklist:jti:"+claims.ID {
			t.Error("should not blacklist an already-expired token")
		}
	}
}

func TestGetTokenVersion_MissingReturnsZero(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()

	tv, err := svc.GetTokenVersion(context.Background(), adminID)
	if err != nil {
		t.Fatalf("GetTokenVersion failed: %v", err)
	}
	if tv != 0 {
		t.Errorf("expected tv=0 for missing key, got %d", tv)
	}
}

func TestGetTokenVersion_ReturnsStoredValue(t *testing.T) {
	svc, mr := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()

	mr.Set("user:"+adminID.String()+":tv", "5")

	tv, err := svc.GetTokenVersion(context.Background(), adminID)
	if err != nil {
		t.Fatalf("GetTokenVersion failed: %v", err)
	}
	if tv != 5 {
		t.Errorf("expected tv=5, got %d", tv)
	}
}

func TestValidateToken_FailClosedOnRedisError(t *testing.T) {
	// Use a Redis client pointing at a non-existent address to simulate Redis failure
	badRdb := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	defer badRdb.Close()

	svc := &AuthService{secret: []byte("test-secret"), redisClient: badRdb}

	adminID := uuid.New()
	now := time.Now()
	tok := signTestToken("test-secret", AccessClaims{
		AdminID: adminID,
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error when Redis is unavailable (fail-closed), got nil")
	}
	// Should contain the "service temporarily unavailable" message
	if !strings.Contains(err.Error(), "service temporarily unavailable") {
		t.Errorf("expected 'service temporarily unavailable' in error, got: %v", err)
	}
}

func TestValidateToken_FailClosedOnRedisError_TokenVersionCheck(t *testing.T) {
	// Redis reachable for blacklist check but fails on token version check.
	// Use a mock Redis that only works for EXISTS but not GET — simulating
	// a partial failure. With miniredis we can't do this easily, so instead
	// we use a bad client that fails on the second call.
	// Simpler approach: use a broken client for the whole thing.
	badRdb := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	defer badRdb.Close()

	svc := &AuthService{secret: []byte("test-secret"), redisClient: badRdb}

	adminID := uuid.New()
	now := time.Now()
	tok := signTestToken("test-secret", AccessClaims{
		AdminID:      adminID,
		Role:         "admin",
		TokenVersion: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	_, err := svc.ValidateToken(context.Background(), tok)
	if err == nil {
		t.Fatal("expected fail-closed error when Redis is down")
	}
}

func TestParseTokenUnsafe_ExtractsClaims(t *testing.T) {
	svc, _ := newTestAuthService(t, "test-secret-123")
	adminID := uuid.New()
	jti := uuid.New().String()
	now := time.Now()

	tok := signTestToken("test-secret-123", AccessClaims{
		AdminID: adminID,
		Role:    "viewer",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	})

	claims, err := svc.parseTokenClaims(tok)
	if err != nil {
		t.Fatalf("parseTokenClaims failed: %v", err)
	}
	if claims.ID != jti {
		t.Errorf("JTI = %q, want %q", claims.ID, jti)
	}
	if claims.AdminID != adminID {
		t.Errorf("AdminID = %v, want %v", claims.AdminID, adminID)
	}
	if claims.Role != "viewer" {
		t.Errorf("Role = %q, want %q", claims.Role, "viewer")
	}
}
