package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/utils"
)

type WeddingEvent struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name                string    `gorm:"size:255;not null" json:"name"`
	Date                time.Time `json:"date"`
	VenueName           string    `gorm:"size:255" json:"venueName"`
	VenueAddress        string    `gorm:"size:500" json:"venueAddress"`
	KioskDescription    string    `gorm:"type:text" json:"kioskDescription"`
	KioskLogoUrl        string    `gorm:"size:500" json:"kioskLogoUrl"`
	KioskBackgroundUrl  string    `gorm:"size:500" json:"kioskBackgroundUrl"`
	KioskBackgroundBlur int       `json:"kioskBackgroundBlur"`
	KioskBackgroundSize string    `gorm:"size:20" json:"kioskBackgroundSize"`
	KioskBackgroundPosX string    `gorm:"size:10" json:"kioskBackgroundPosX"`
	KioskBackgroundPosY string    `gorm:"size:10" json:"kioskBackgroundPosY"`
	KioskLogoSize       string    `gorm:"size:20" json:"kioskLogoSize"`
	KioskLogoPosX       string    `gorm:"size:10" json:"kioskLogoPosX"`
	KioskLogoPosY       string    `gorm:"size:10" json:"kioskLogoPosY"`
	ShowSeatNumbers     bool      `gorm:"default:true" json:"showSeatNumbers"`
	HallWidth           int       `gorm:"not null;default:860" json:"hallWidth"`
	HallHeight          int       `gorm:"not null;default:1000" json:"hallHeight"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func (w WeddingEvent) MarshalJSON() ([]byte, error) {
	type Alias WeddingEvent
	return json.Marshal(&struct {
		ID string `json:"id"`
		Alias
	}{
		ID:    utils.EncodeUUID(w.ID),
		Alias: (Alias)(w),
	})
}
