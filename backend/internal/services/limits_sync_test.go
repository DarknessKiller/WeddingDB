package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSyncCreate_LimitReasons proves an offline create that trips a limit cap
// reports why it failed instead of a generic "Internal error", so the client
// can show the offending field.
func TestSyncCreate_LimitReasons(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	ctx := context.Background()

	dietary := make([]string, MaxDietaryItems+1)
	for i := range dietary {
		dietary[i] = "vegan"
	}

	for _, tc := range []struct {
		name    string
		payload *SyncPayload
		reason  string
	}{
		{"party size", &SyncPayload{Name: "Big", Pax: MaxGuestPartySize + 1}, "party size"},
		{"dietary", &SyncPayload{Name: "Pick", Pax: 1, Dietary: dietary}, "dietary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Sync(ctx, wid, []SyncMutation{{
				Op:              SyncOpCreate,
				GuestID:         uuid.New().String(),
				ClientUpdatedAt: time.Now(),
				Payload:         tc.payload,
			}})
			if err != nil {
				t.Fatalf("sync: %v", err)
			}
			if len(res) != 1 || res[0].Status != "failed" {
				t.Fatalf("sync create over %s should fail, got %+v", tc.name, res)
			}
			if !strings.Contains(res[0].Reason, tc.reason) {
				t.Fatalf("reason = %q, want it to contain %q", res[0].Reason, tc.reason)
			}
		})
	}
}
