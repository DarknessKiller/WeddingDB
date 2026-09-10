package handlers

import (
	"encoding/json"
	"testing"
)

func TestPublicGuestJSONOmitsContactFields(t *testing.T) {
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
	for _, field := range []string{"phone", "email"} {
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
