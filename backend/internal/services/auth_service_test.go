package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

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
