package services

import (
	"context"
	"testing"
)

// TestLogin_UnknownEmailRunsBcrypt guards against user enumeration by timing:
// an unknown email must still pay for one bcrypt comparison.
func TestLogin_UnknownEmailRunsBcrypt(t *testing.T) {
	svc, _, _ := newIntegrationAuthService(t, "secret")
	calls := 0
	orig := compareHash
	compareHash = func(h, p []byte) error { calls++; return orig(h, p) }
	t.Cleanup(func() { compareHash = orig })

	if _, err := svc.Login(context.Background(), "nobody@example.test", "whatever"); err == nil {
		t.Fatal("expected invalid credentials")
	}
	if calls != 1 {
		t.Fatalf("bcrypt compares on unknown email = %d, want 1", calls)
	}
}
