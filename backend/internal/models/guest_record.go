package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

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

type GuestRecord struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID   uuid.UUID   `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name        string      `gorm:"size:255;not null" json:"name"`
	NamePinyin  string      `gorm:"size:255" json:"-"`
	Phone       string      `gorm:"size:50" json:"phone"`
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
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}
