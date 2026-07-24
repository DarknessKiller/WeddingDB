package models

import (
	"time"

	"github.com/google/uuid"
)

type BanquetTable struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Capacity  int       `gorm:"not null" json:"capacity"`
	Row       int       `gorm:"not null;default:1" json:"row"`
	Col       int       `gorm:"not null;default:1" json:"col"`
	IsVip     bool      `json:"isVip"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
