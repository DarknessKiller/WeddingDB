package models

import (
	"time"

	"github.com/google/uuid"
)

type WeddingEvent struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name                string    `gorm:"size:255;not null" json:"name"`
	Date                time.Time `json:"date"`
	KioskTitle          string    `gorm:"size:255" json:"kioskTitle"`
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
	HallWidth           int       `gorm:"not null;default:860" json:"hallWidth"`
	HallHeight          int       `gorm:"not null;default:1000" json:"hallHeight"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
