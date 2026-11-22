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

// FieldTimes stores the last write time of each guest column group as JSON
// text, using the same pattern as StringSlice.
type FieldTimes map[string]time.Time

func (f FieldTimes) Value() (driver.Value, error) {
	if f == nil {
		return "{}", nil
	}
	return json.Marshal(f)
}

func (f *FieldTimes) Scan(src any) error {
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
		return fmt.Errorf("cannot scan %T into FieldTimes", src)
	}
}

type GuestRecord struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID   uuid.UUID   `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name        string      `gorm:"size:255;not null" json:"name"`
	NamePinyin  string      `gorm:"size:255" json:"-"`
	Email       string      `gorm:"size:255" json:"email"`
	Pax         int         `gorm:"not null;default:1" json:"pax"`
	TableID     *uuid.UUID  `gorm:"type:uuid;index" json:"tableId"`
	SeatNum     *int        `json:"seatNum"`
	RSVP        string      `gorm:"size:20;default:no_response" json:"rsvp"`
	CheckedInAt *time.Time  `json:"checkedInAt"`
	Notes       string      `gorm:"type:text" json:"notes"`
	Dietary     StringSlice `gorm:"type:text" json:"dietary"`
	IsVip       bool        `json:"isVip"`
	AngbaoAmt   *int        `json:"angbaoAmt"`
	GiftItem    *string     `gorm:"size:255" json:"giftItem"`
	FieldTimes  FieldTimes  `gorm:"type:text" json:"-"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
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
