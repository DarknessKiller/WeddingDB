package services

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_LongPasswordRoundTrips(t *testing.T) {
	// 72 characters, 208 bytes: over bcrypt's limit, inside the form's.
	long := "Aa1!" + strings.Repeat("密", 68)
	hash, err := HashPassword(long)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, sha256Prefix) {
		t.Fatalf("long password hash %q lacks the pre-hash marker", hash)
	}
	if err := ComparePassword(hash, long); err != nil {
		t.Errorf("correct long password rejected: %v", err)
	}
	if err := ComparePassword(hash, long+"x"); err == nil {
		t.Error("wrong long password accepted")
	}
}

func TestHashPassword_ShortPasswordStaysBcrypt(t *testing.T) {
	hash, err := HashPassword("OldPassw0rd!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if strings.HasPrefix(hash, sha256Prefix) {
		t.Fatalf("short password hash %q was pre-hashed", hash)
	}
	if err := ComparePassword(hash, "OldPassw0rd!"); err != nil {
		t.Errorf("correct password rejected: %v", err)
	}
}

func TestComparePassword_AcceptsExistingBcryptHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("OldPassw0rd!"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("fixture hash: %v", err)
	}
	if err := ComparePassword(string(hash), "OldPassw0rd!"); err != nil {
		t.Errorf("hash made before the pre-hash change no longer verifies: %v", err)
	}
	if err := ComparePassword(string(hash), "WrongPassw0rd!"); err == nil {
		t.Error("wrong password accepted")
	}
}
