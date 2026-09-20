package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/utils"
)

func TestErrAlreadyCheckedIn_IsSentinel(t *testing.T) {
	if ErrAlreadyCheckedIn == nil {
		t.Fatal("ErrAlreadyCheckedIn should not be nil")
	}
	if ErrAlreadyCheckedIn.Error() != "guest already checked in" {
		t.Errorf("error message = %q", ErrAlreadyCheckedIn.Error())
	}
}

func TestErrAlreadyCheckedIn_WrapsCorrectly(t *testing.T) {
	// Verify errors.Is works with wrapped errors.
	wrapped := errors.New("context: ") //nolint:err113
	err := errors.Join(wrapped, ErrAlreadyCheckedIn)
	if !errors.Is(err, ErrAlreadyCheckedIn) {
		t.Error("errors.Is should match ErrAlreadyCheckedIn through Join")
	}
}

func TestGuestToEventData_FullGuest(t *testing.T) {
	gid := uuid.New()
	tableID := uuid.New()
	seatNum := 5
	now := time.Now()
	angbao := 888
	gift := "gold ring"

	guest := &models.GuestRecord{
		ID:          gid,
		Name:        "张三",
		Phone:       "13800138000",
		Email:       "zhang@example.com",
		Pax:         4,
		RSVP:        "confirmed",
		IsVip:       true,
		Notes:       "VIP table",
		Dietary:     []string{"no seafood"},
		TableID:     &tableID,
		SeatNum:     &seatNum,
		CheckedInAt: &now,
		AngbaoAmt:   &angbao,
		GiftItem:    &gift,
	}

	d := guestToEventData(guest)

	if d.ID != utils.EncodeUUID(gid) {
		t.Errorf("ID = %q, want %q (base64-encoded)", d.ID, utils.EncodeUUID(gid))
	}
	if d.Name != "张三" {
		t.Errorf("Name = %q, want %q", d.Name, "张三")
	}
	if d.Pax != 4 {
		t.Errorf("Pax = %d, want 4", d.Pax)
	}
	if !d.IsVip {
		t.Error("IsVip should be true")
	}
	if d.TableID == nil || *d.TableID != utils.EncodeUUID(tableID) {
		t.Errorf("TableID = %v, want %v (base64-encoded)", d.TableID, utils.EncodeUUID(tableID))
	}
	if d.SeatNum == nil || *d.SeatNum != 5 {
		t.Errorf("SeatNum = %v, want 5", d.SeatNum)
	}
	if d.CheckedInAt == nil {
		t.Error("CheckedInAt should not be nil")
	}
	if d.AngbaoAmt == nil || *d.AngbaoAmt != 888 {
		t.Errorf("AngbaoAmt = %v, want 888", d.AngbaoAmt)
	}
	if d.GiftItem == nil || *d.GiftItem != "gold ring" {
		t.Errorf("GiftItem = %v, want %q", d.GiftItem, "gold ring")
	}
	if len(d.Dietary) != 1 || d.Dietary[0] != "no seafood" {
		t.Errorf("Dietary = %v", d.Dietary)
	}
}

func TestGuestToEventData_NilOptionalFields(t *testing.T) {
	guest := &models.GuestRecord{
		ID:   uuid.New(),
		Name: "Alice",
		Pax:  1,
	}

	d := guestToEventData(guest)

	if d.TableID != nil {
		t.Error("TableID should be nil")
	}
	if d.SeatNum != nil {
		t.Error("SeatNum should be nil")
	}
	if d.CheckedInAt != nil {
		t.Error("CheckedInAt should be nil")
	}
	if d.AngbaoAmt != nil {
		t.Error("AngbaoAmt should be nil")
	}
	if d.GiftItem != nil {
		t.Error("GiftItem should be nil")
	}
}

func TestGuestEvent_Serialization(t *testing.T) {
	event := GuestEvent{
		Type:      "checkin",
		GuestID:   uuid.New().String(),
		WeddingID: uuid.New().String(),
		Guest: &GuestEventData{
			ID:   uuid.New().String(),
			Name: "Bob",
			Pax:  2,
		},
		Timestamp: 1234567890,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed GuestEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Type != "checkin" {
		t.Errorf("Type = %q, want %q", parsed.Type, "checkin")
	}
	if parsed.Guest == nil {
		t.Fatal("Guest is nil")
	}
	if parsed.Guest.Name != "Bob" {
		t.Errorf("Guest.Name = %q, want %q", parsed.Guest.Name, "Bob")
	}
}

// TestCheckInPromotesRSVPAndAppendsNotes covers both check-in paths: the
// online endpoint and the offline sync op. Arrival must promote RSVP to
// confirmed and never wipe existing notes.
func TestCheckInPromotesRSVPAndAppendsNotes(t *testing.T) {
	ctx := context.Background()

	t.Run("online endpoint", func(t *testing.T) {
		svc, db, wid := newGuestSyncService(t)
		gid := uuid.New()
		seed := &models.GuestRecord{ID: gid, WeddingID: wid, Name: "Pending Guest", Pax: 1, RSVP: "pending", Notes: "vegetarian", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}

		if err := svc.CheckIn(ctx, gid, wid, " came with 2 kids "); err != nil {
			t.Fatalf("checkin: %v", err)
		}

		var got models.GuestRecord
		if err := db.First(&got, "id = ?", gid).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.RSVP != "confirmed" {
			t.Errorf("RSVP = %q, want confirmed", got.RSVP)
		}
		if got.CheckedInAt == nil {
			t.Error("CheckedInAt is nil")
		}
		if got.Notes != "vegetarian\ncame with 2 kids" {
			t.Errorf("Notes = %q", got.Notes)
		}

		// FIFO conflict: a second check-in must not overwrite the first note.
		if err := svc.CheckIn(ctx, gid, wid, "late note"); !errors.Is(err, ErrAlreadyCheckedIn) {
			t.Errorf("second checkin err = %v, want ErrAlreadyCheckedIn", err)
		}
	})

	t.Run("offline sync op", func(t *testing.T) {
		svc, db, wid := newGuestSyncService(t)
		gid := uuid.New()
		seed := &models.GuestRecord{ID: gid, WeddingID: wid, Name: "Pending Guest", Pax: 1, RSVP: "no_response", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}

		res, err := svc.Sync(ctx, wid, []SyncMutation{{
			MutationID:      "m1",
			Op:              SyncOpCheckIn,
			GuestID:         gid.String(),
			ClientUpdatedAt: time.Now(),
			Payload:         &SyncPayload{Notes: "no gift"},
		}})
		if err != nil {
			t.Fatalf("sync: %v", err)
		}
		if len(res) != 1 || res[0].Status != "applied" {
			t.Fatalf("sync result = %+v", res)
		}

		var got models.GuestRecord
		if err := db.First(&got, "id = ?", gid).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.RSVP != "confirmed" {
			t.Errorf("RSVP = %q, want confirmed", got.RSVP)
		}
		if got.Notes != "no gift" {
			t.Errorf("Notes = %q", got.Notes)
		}
	})
}
