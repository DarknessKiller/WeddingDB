package middleware

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

func TestDecodeWIDString_StandardUUID(t *testing.T) {
	orig := uuid.New()
	got, err := DecodeWIDString(orig.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != orig {
		t.Errorf("got %v, want %v", got, orig)
	}
}

func TestDecodeWIDString_Base64Encoded(t *testing.T) {
	orig := uuid.New()
	b64 := base64.StdEncoding.EncodeToString(orig[:])
	got, err := DecodeWIDString(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != orig {
		t.Errorf("got %v, want %v", got, orig)
	}
}

func TestDecodeWIDString_URLSafeBase64(t *testing.T) {
	orig := uuid.New()
	b64 := base64.URLEncoding.EncodeToString(orig[:])
	got, err := DecodeWIDString(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != orig {
		t.Errorf("got %v, want %v", got, orig)
	}
}

func TestDecodeWIDString_PaddedBase64(t *testing.T) {
	orig := uuid.New()
	b64 := base64.RawStdEncoding.EncodeToString(orig[:])
	got, err := DecodeWIDString(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != orig {
		t.Errorf("got %v, want %v", got, orig)
	}
}

func TestDecodeWIDString_InvalidString(t *testing.T) {
	_, err := DecodeWIDString("not-a-uuid-or-base64")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestDecodeWIDString_EmptyString(t *testing.T) {
	_, err := DecodeWIDString("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}
