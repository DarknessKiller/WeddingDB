package services

import (
	"fmt"
	"unicode/utf8"

	"weddingdb/internal/models"
)

// The frontend mirrors these caps in frontend/src/lib/utils/limits.ts
// (LIMITS); a backend cap change belongs in both files and in the form field
// that feeds it (CODING_STANDARDS.md, "Frontend input limits").
const (
	MaxNameLen             = 255
	MaxEmailLen            = 254
	MaxPasswordLen         = 72 // bcrypt's byte ceiling; passwords are ASCII only
	MaxNotesLen            = 2000
	MaxCheckInNoteLen      = 500
	MaxDietaryItemLen      = 100
	MaxDietaryItems        = 20
	MaxGiftItemLen         = 255
	MaxTableNameLen        = 100
	MaxVenueNameLen        = 255
	MaxVenueAddressLen     = 500
	MaxKioskDescriptionLen = 500
	MaxURLLen              = 500
	MaxColorLen            = 20
)

// FieldLenError reports a text field over its cap. Handlers map it to a 400.
type FieldLenError struct {
	Field string
	Max   int
}

func (e FieldLenError) Error() string {
	return fmt.Sprintf("%s must be at most %d characters", e.Field, e.Max)
}

func checkLen(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return FieldLenError{Field: field, Max: max}
	}
	return nil
}

// ValidateGuestText enforces the caps for every guest write path: online
// create/update, bulk import and offline sync.
func ValidateGuestText(g *models.GuestRecord) error {
	if err := checkLen("Name", g.Name, MaxNameLen); err != nil {
		return err
	}
	if err := checkLen("Email", g.Email, MaxEmailLen); err != nil {
		return err
	}
	if err := checkLen("Notes", g.Notes, MaxNotesLen); err != nil {
		return err
	}
	if g.GiftItem != nil {
		if err := checkLen("Gift item", *g.GiftItem, MaxGiftItemLen); err != nil {
			return err
		}
	}
	if len(g.Dietary) > MaxDietaryItems {
		return fmt.Errorf("dietary can list at most %d items", MaxDietaryItems)
	}
	for _, item := range g.Dietary {
		if err := checkLen("Dietary item", item, MaxDietaryItemLen); err != nil {
			return err
		}
	}
	return nil
}

func ValidateTableName(name string) error {
	return checkLen("Name", name, MaxTableNameLen)
}

func ValidateWeddingText(w *models.WeddingEvent) error {
	for _, f := range []struct {
		field string
		value string
		max   int
	}{
		{"Name", w.Name, MaxNameLen},
		{"Venue name", w.VenueName, MaxVenueNameLen},
		{"Venue address", w.VenueAddress, MaxVenueAddressLen},
		{"Kiosk description", w.KioskDescription, MaxKioskDescriptionLen},
		{"Kiosk logo URL", w.KioskLogoUrl, MaxURLLen},
		{"Kiosk background URL", w.KioskBackgroundUrl, MaxURLLen},
	} {
		if err := checkLen(f.field, f.value, f.max); err != nil {
			return err
		}
	}
	return nil
}

func ValidateElementText(e *models.HallElement) error {
	for _, f := range []struct {
		field string
		value string
		max   int
	}{
		{"Element name", e.Name, MaxNameLen},
		{"Element color", e.Color, MaxColorLen},
		{"Element text color", e.TextColor, MaxColorLen},
		{"Element stroke color", e.StrokeColor, MaxColorLen},
	} {
		if err := checkLen(f.field, f.value, f.max); err != nil {
			return err
		}
	}
	return nil
}

func ValidateAdminText(name, email string) error {
	if err := checkLen("Name", name, MaxNameLen); err != nil {
		return err
	}
	return checkLen("Email", email, MaxEmailLen)
}
