package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPublicGuestJSONOmitsPrivateFields(t *testing.T) {
	data, err := json.Marshal(publicGuest{
		ID:   "guest-id",
		Name: "Alice",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"phone", "email", "checkedInAt"} {
		if _, present := fields[field]; present {
			t.Errorf("public guest JSON contains %q: %s", field, data)
		}
	}
}

// TestPublicGuestJSONExposesKioskFields covers fix 8: the kiosk UI and
// frontend/api/public.ts expect isVip and checkedInAt on the public payload.
func TestPublicGuestJSONExposesKioskFields(t *testing.T) {
	data, err := json.Marshal(publicGuest{
		ID:    "guest-id",
		Name:  "Alice",
		IsVip: true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if vip, ok := fields["isVip"].(bool); !ok || !vip {
		t.Errorf("public guest JSON should expose isVip=true: %s", data)
	}
}

// A byte-slice cap splits multibyte runes and hands Postgres invalid UTF-8.
func TestLimitQuery_KeepsValidUTF8(t *testing.T) {
	got := limitQuery(strings.Repeat("婚", 150), 100)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated query is invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 100 {
		t.Fatalf("rune count = %d, want 100", n)
	}
	if short := limitQuery("abc", 100); short != "abc" {
		t.Fatalf("short query changed to %q", short)
	}
}
