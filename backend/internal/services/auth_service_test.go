package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// mockRevoker is an in-memory revoker for unit tests.
type mockRevoker struct {
	mu       sync.Mutex
	revoked  map[string]bool
	revokeFn func(ctx context.Context, jti string, ttl time.Duration) error
}

func newMockRevoker() *mockRevoker {
	return &mockRevoker{revoked: make(map[string]bool)}
}

func (m *mockRevoker) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, jti, ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revoked[jti] = true
	return nil
}

func (m *mockRevoker) IsRevoked(ctx context.Context, jti string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revoked[jti]
}

// newTestAuthService creates an AuthService with nil repos (ValidateToken doesn't need them).
func newTestAuthService(secret string) *AuthService {
	return &AuthService{secret: []byte(secret)}
}

func signTestToken(secret string, claims AccessClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestValidateToken_Valid(t *testing.T) {
	svc := newTestAuthService("test-secret-123")
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

	claims, err := svc.ValidateToken(tok)
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
	svc := newTestAuthService("test-secret-123")
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

	_, err := svc.ValidateToken(tok)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	svc := newTestAuthService("correct-secret")
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

	_, err := svc.ValidateToken(tok)
	if err == nil {
		t.Fatal("expected error for wrong-secret token, got nil")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	svc := newTestAuthService("test-secret-123")

	_, err := svc.ValidateToken("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

func TestValidateToken_Revoked(t *testing.T) {
	secret := "test-secret-123"
	revoker := newMockRevoker()
	svc := &AuthService{secret: []byte(secret), revoker: revoker}

	jti := uuid.New().String()
	tok := signTestToken(secret, AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})

	// Token is valid before revocation
	_, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("expected valid token before revocation, got: %v", err)
	}

	// Revoke it
	if err := revoker.Revoke(context.Background(), jti, 15*time.Minute); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// Now ValidateToken should reject it
	_, err = svc.ValidateToken(tok)
	if err == nil {
		t.Fatal("expected error for revoked token, got nil")
	}
	if err.Error() != "token revoked" {
		t.Errorf("error = %q, want %q", err.Error(), "token revoked")
	}
}

func TestValidateToken_NilRevoker(t *testing.T) {
	secret := "test-secret-123"
	svc := &AuthService{secret: []byte(secret), revoker: nil}

	tok := signTestToken(secret, AccessClaims{
		AdminID: uuid.New(),
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})

	claims, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
}

func TestValidateToken_NilWeddingID(t *testing.T) {
	svc := newTestAuthService("test-secret-123")
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

	claims, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.WeddingID != nil {
		t.Errorf("expected nil WeddingID, got %v", claims.WeddingID)
	}
}
