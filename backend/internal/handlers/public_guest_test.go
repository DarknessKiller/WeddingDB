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
