package models

import (
	"time"

	"github.com/google/uuid"
)

type BanquetTable struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Capacity  int       `gorm:"not null" json:"capacity"`
	X         float64   `gorm:"not null;default:0" json:"x"`
	Y         float64   `gorm:"not null;default:0" json:"y"`
	Degree    float64   `gorm:"not null;default:0" json:"degree"`
	IsVip     bool      `json:"isVip"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
