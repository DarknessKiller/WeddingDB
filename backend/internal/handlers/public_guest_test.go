package handlers

import (
	"encoding/json"
	"testing"
	"time"
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
	checkedIn := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	data, err := json.Marshal(publicGuest{
		ID:          "guest-id",
		Name:        "Alice",
		IsVip:       true,
		CheckedInAt: &checkedIn,
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
	if at, ok := fields["checkedInAt"].(string); !ok || at != "2024-05-01T12:00:00Z" {
		t.Errorf("public guest JSON should expose checkedInAt as RFC3339: %s", data)
	}
}
