package services

import (
	"context"
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
