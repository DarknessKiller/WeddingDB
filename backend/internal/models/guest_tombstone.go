package models

import (
	"time"

	"github.com/google/uuid"
)

// GuestTombstone records a deleted guest's ID, so an offline create replayed
// after the delete (its first response was lost) does not bring the guest back.
type GuestTombstone struct {
	GuestID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null"`
	CreatedAt time.Time
}
