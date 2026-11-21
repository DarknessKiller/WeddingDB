package services

import (
	"strings"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"
)

func TestValidateGuestText_Bounds(t *testing.T) {
	at := strings.Repeat("x", MaxNameLen)
	over := at + "x"

	testutil.NoErr(t, ValidateGuestText(&models.GuestRecord{Name: at}))
	if err := ValidateGuestText(&models.GuestRecord{Name: over}); err == nil {
		t.Fatal("name over the cap should be rejected")
	}

	long := strings.Repeat("x", MaxGiftItemLen+1)
	if err := ValidateGuestText(&models.GuestRecord{Name: "a", GiftItem: &long}); err == nil {
		t.Fatal("gift item over the cap should be rejected")
	}

	many := make([]string, MaxDietaryItems+1)
	for i := range many {
		many[i] = "vegetarian"
	}
	if err := ValidateGuestText(&models.GuestRecord{Name: "a", Dietary: many}); err == nil {
		t.Fatal("dietary list over the cap should be rejected")
	}
}

func TestValidateWeddingText_Bounds(t *testing.T) {
	w := &models.WeddingEvent{Name: strings.Repeat("x", MaxNameLen)}
	testutil.NoErr(t, ValidateWeddingText(w))
	w.VenueAddress = strings.Repeat("x", MaxVenueAddressLen+1)
	if err := ValidateWeddingText(w); err == nil {
		t.Fatal("venue address over the cap should be rejected")
	}
}

func TestValidateElementText_Bounds(t *testing.T) {
	el := &models.HallElement{Name: "Stage", Color: "#000000"}
	testutil.NoErr(t, ValidateElementText(el))
	el.Color = strings.Repeat("c", MaxColorLen+1)
	if err := ValidateElementText(el); err == nil {
		t.Fatal("element color over the cap should be rejected")
	}
}

func TestValidateTableAndAdminText(t *testing.T) {
	testutil.NoErr(t, ValidateTableName(strings.Repeat("x", MaxTableNameLen)))
	if err := ValidateTableName(strings.Repeat("x", MaxTableNameLen+1)); err == nil {
		t.Fatal("table name over the cap should be rejected")
	}
	testutil.NoErr(t, ValidateAdminText("Alice", "alice@example.com"))
	if err := ValidateAdminText(strings.Repeat("x", MaxNameLen+1), "alice@example.com"); err == nil {
		t.Fatal("admin name over the cap should be rejected")
	}
}
