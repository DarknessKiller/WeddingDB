package utils

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

func TestEncodeUUID_RoundTrip(t *testing.T) {
	orig := uuid.New()
	encoded := EncodeUUID(orig)

	// Must be base64 raw URL-safe encoded (no padding)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("encoded string is not valid raw URL-safe base64: %v", err)
	}

	got, err := uuid.FromBytes(decoded)
	if err != nil {
		t.Fatalf("decoded bytes are not a valid UUID: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip failed: got %v, want %v", got, orig)
	}
}

func TestEncodeUUID_NoPadding(t *testing.T) {
	orig := uuid.New()
	encoded := EncodeUUID(orig)
	if encoded[len(encoded)-1] == '=' {
		t.Error("EncodeUUID output should not have padding")
	}
}

func TestEncodeUUID_Length(t *testing.T) {
	orig := uuid.New()
	encoded := EncodeUUID(orig)
	// UUID is 16 bytes → raw base64 is ceil(16*4/3) = 22 chars
	if len(encoded) != 22 {
		t.Errorf("expected 22 chars, got %d: %q", len(encoded), encoded)
	}
}
