package services

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/utils"
)

func TestErrAlreadyCheckedIn_IsSentinel(t *testing.T) {
	if models.ErrAlreadyCheckedIn == nil {
		t.Fatal("ErrAlreadyCheckedIn should not be nil")
	}
	if models.ErrAlreadyCheckedIn.Error() != "guest already checked in" {
		t.Errorf("error message = %q", models.ErrAlreadyCheckedIn.Error())
	}
}

func TestErrAlreadyCheckedIn_WrapsCorrectly(t *testing.T) {
	// Verify errors.Is works with wrapped errors.
	wrapped := errors.New("context: ") //nolint:err113
	err := errors.Join(wrapped, models.ErrAlreadyCheckedIn)
	if !errors.Is(err, models.ErrAlreadyCheckedIn) {
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
