package models

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AdminID   uuid.UUID  `gorm:"type:uuid;index;not null"`
	WeddingID *uuid.UUID `gorm:"type:uuid"`
	Token     string     `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
	// RotatedAt marks a consumed token; ReplacedBy names the successor it was
	// rotated into. Both stay set so a retry inside the grace window can be
	// answered idempotently instead of ending the session.
	RotatedAt  *time.Time `gorm:"index"`
	ReplacedBy *string    `gorm:"size:255"`
}
