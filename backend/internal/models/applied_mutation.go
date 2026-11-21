package models

import (
	"time"

	"github.com/google/uuid"
)

// AppliedMutation marks an offline-sync mutation the server already applied,
// so a replay of the same mutationId changes nothing.
type AppliedMutation struct {
	WeddingID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	MutationID string    `gorm:"size:64;primaryKey"`
	CreatedAt  time.Time
}
