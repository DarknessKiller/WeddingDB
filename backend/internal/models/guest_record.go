package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"weddingdb/internal/utils"

	"github.com/google/uuid"
)

// StringSlice stores a []string as JSON text in a text column.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(src interface{}) error {
	if src == nil {
		*s = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("cannot scan %T into StringSlice", src)
	}
}

// Guest field keys for GuestFieldTimes, as column names.
const (
	FieldName      = "name"
	FieldEmail     = "email"
	FieldPax       = "pax"
	FieldRSVP      = "rsvp"
	FieldNotes     = "notes"
	FieldDietary   = "dietary"
	FieldIsVip     = "is_vip"
	FieldAngbaoAmt = "angbao_amt"
	FieldGiftItem  = "gift_item"
	FieldSeat      = "table_id" // table_id and seat_num move together
	FieldCheckedIn = "checked_in_at"
)

// GuestEditFields are the fields a full-row guest write carries. checked_in_at
// is not one: the check-in and check-out paths own it.
var GuestEditFields = []string{
	FieldName, FieldEmail, FieldPax, FieldRSVP, FieldNotes,
	FieldDietary, FieldIsVip, FieldAngbaoAmt, FieldGiftItem, FieldSeat,
}

// GuestFieldTimes records when each guest field was last written, keyed by
// column name. Per-field last-write-wins needs it: one row updated_at cannot
// say which fields a write carried.
type GuestFieldTimes map[string]time.Time

func (f GuestFieldTimes) Value() (driver.Value, error) {
	if f == nil {
		return "{}", nil
	}
	return json.Marshal(f)
}

func (f *GuestFieldTimes) Scan(src interface{}) error {
	if src == nil {
		*f = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, f)
	case string:
		return json.Unmarshal([]byte(v), f)
	default:
		return fmt.Errorf("cannot scan %T into GuestFieldTimes", src)
	}
}

type GuestRecord struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID      uuid.UUID       `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name           string          `gorm:"size:255;not null" json:"name"`
	NamePinyin     string          `gorm:"type:text" json:"-"`
	Email          string          `gorm:"size:255" json:"email"`
	Pax            int             `gorm:"not null;default:1" json:"pax"`
	TableID        *uuid.UUID      `gorm:"type:uuid;index" json:"tableId"`
	SeatNum        *int            `json:"seatNum"`
	RSVP           string          `gorm:"size:20;default:no_response" json:"rsvp"`
	CheckedInAt    *time.Time      `json:"checkedInAt"`
	Notes          string          `gorm:"type:text" json:"notes"`
	Dietary        StringSlice     `gorm:"type:text" json:"dietary"`
	IsVip          bool            `json:"isVip"`
	AngbaoAmt      *int            `json:"angbaoAmt"`
	GiftItem       *string         `gorm:"size:255" json:"giftItem"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	FieldUpdatedAt GuestFieldTimes `gorm:"type:jsonb" json:"-"`
}

// WrittenAt reports when field was last written. A row written before field
// times were recorded falls back to the row time, but only for a field that
// holds a value: an unset field has no write to lose to, so its first write
// wins. checked_in_at falls back to its own value, the arrival write.
func (g *GuestRecord) WrittenAt(field string) time.Time {
	if t, ok := g.FieldUpdatedAt[field]; ok {
		return t
	}
	if field == FieldCheckedIn && g.CheckedInAt != nil {
		return *g.CheckedInAt
	}
	if !g.fieldSet(field) {
		return time.Time{}
	}
	return g.UpdatedAt
}

// fieldSet reports whether the column holds a value, for rows with no field
// times: a nil angbao was never keyed, so no write time should block it.
func (g *GuestRecord) fieldSet(field string) bool {
	switch field {
	case FieldEmail:
		return g.Email != ""
	case FieldPax:
		return g.Pax != 0
	case FieldRSVP:
		return g.RSVP != ""
	case FieldNotes:
		return g.Notes != ""
	case FieldDietary:
		return len(g.Dietary) > 0
	case FieldIsVip:
		return g.IsVip
	case FieldAngbaoAmt:
		return g.AngbaoAmt != nil
	case FieldGiftItem:
		return g.GiftItem != nil
	case FieldSeat:
		return g.TableID != nil || g.SeatNum != nil
	case FieldCheckedIn:
		return g.CheckedInAt != nil
	}
	return true
}

// StampField records at as the write time of field, keeping the newest.
func (g *GuestRecord) StampField(field string, at time.Time) {
	if !at.After(g.WrittenAt(field)) {
		return
	}
	if g.FieldUpdatedAt == nil {
		g.FieldUpdatedAt = make(GuestFieldTimes, 1)
	}
	g.FieldUpdatedAt[field] = at
}

// StampFields records at as the write time of every field in fields.
func (g *GuestRecord) StampFields(at time.Time, fields []string) {
	for _, f := range fields {
		g.StampField(f, at)
	}
}

func (g GuestRecord) MarshalJSON() ([]byte, error) {
	type Alias GuestRecord
	var tid *string
	if g.TableID != nil {
		s := utils.EncodeUUID(*g.TableID)
		tid = &s
	}
	return json.Marshal(&struct {
		ID        string  `json:"id"`
		WeddingID string  `json:"weddingId"`
		TableID   *string `json:"tableId"`
		Pinyin    string  `json:"pinyin"`
		Alias
	}{
		ID:        utils.EncodeUUID(g.ID),
		WeddingID: utils.EncodeUUID(g.WeddingID),
		TableID:   tid,
		Pinyin:    g.NamePinyin,
		Alias:     (Alias)(g),
	})
}
