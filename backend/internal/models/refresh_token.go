package models

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AdminID   uuid.UUID `gorm:"type:uuid;index;not null"`
	Token     string    `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}
