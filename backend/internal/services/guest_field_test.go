package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"

	"github.com/google/uuid"
)

// Unbounded guest text must be rejected at the service boundary, otherwise one
// row can carry arbitrary bytes into storage and the public kiosk payload.
func TestCreate_RejectsOversizedFields(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	ctx := context.Background()

	long := strings.Repeat("名", MaxGuestNameLen+1)
	err := svc.Create(ctx, &models.GuestRecord{WeddingID: wid, Name: long, Pax: 1})
	if !errors.Is(err, ErrInvalidGuestField) {
		t.Fatalf("create with %d-char name = %v, want ErrInvalidGuestField", MaxGuestNameLen+1, err)
	}

	cases := []struct {
		name  string
		guest *models.GuestRecord
	}{
		{"email", &models.GuestRecord{Name: "A", Pax: 1, Email: strings.Repeat("a", MaxGuestEmailLen+1)}},
		{"notes", &models.GuestRecord{Name: "A", Pax: 1, Notes: strings.Repeat("n", MaxGuestNotesLen+1)}},
		{"dietary rows", &models.GuestRecord{Name: "A", Pax: 1, Dietary: make([]string, MaxGuestDietaryItems+1)}},
		{"dietary item", &models.GuestRecord{Name: "A", Pax: 1, Dietary: []string{strings.Repeat("d", MaxGuestDietaryItemLen+1)}}},
		{"gift item", &models.GuestRecord{Name: "A", Pax: 1, GiftItem: ptr(strings.Repeat("g", MaxGuestGiftItemLen+1))}},
		{"negative angbao", &models.GuestRecord{Name: "A", Pax: 1, AngbaoAmt: ptr(-1)}},
	}
	for _, tc := range cases {
		tc.guest.WeddingID = wid
		if err := svc.Create(ctx, tc.guest); !errors.Is(err, ErrInvalidGuestField) {
			t.Errorf("create with oversized %s = %v, want ErrInvalidGuestField", tc.name, err)
		}
	}
}

// Check-in appends notes; the append must not push the row past the notes cap.
func TestCheckIn_RejectsNotesOverCap(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)
	ctx := context.Background()

	guest := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Alice", Pax: 1}
	if err := svc.Create(ctx, guest); err != nil {
		t.Fatalf("seed guest: %v", err)
	}
	err := svc.CheckIn(ctx, guest.ID, wid, strings.Repeat("x", MaxGuestNotesLen+1))
	if !errors.Is(err, ErrInvalidGuestField) {
		t.Fatalf("check-in with oversized notes = %v, want ErrInvalidGuestField", err)
	}
}

// The offline path must reject the same fields, reporting the mutation as
// failed so the client keeps it queued instead of silently storing it.
func TestSync_RejectsInvalidFields(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)

	results, err := svc.Sync(context.Background(), wid, []SyncMutation{
		{
			MutationID:      "m1",
			Op:              SyncOpCreate,
			GuestID:         uuid.New().String(),
			ClientUpdatedAt: time.Now(),
			Payload:         &SyncPayload{Name: ptr("Alice"), Pax: ptr(1), AngbaoAmt: NullableJSON[int]{Set: true, Value: ptr(-5)}},
		},
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("sync result = %+v, want one failed mutation", results)
	}
}

func ptr[T any](v T) *T { return &v }
