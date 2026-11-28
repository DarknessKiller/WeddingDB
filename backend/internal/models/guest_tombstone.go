package models

import (
	"time"

	"github.com/google/uuid"
)

// GuestTombstone records a guest delete, so a replayed offline create that is
// older than the delete cannot bring the guest back.
type GuestTombstone struct {
	GuestID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null"`
	DeletedAt time.Time `gorm:"not null"`
}
