package models

import (
	"time"

	"github.com/google/uuid"
)

// UserWedding is the join table for the many-to-many relationship between AdminUser and WeddingEvent.
type UserWedding struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_user_wedding;not null" json:"userId"`
	WeddingID uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_user_wedding;not null" json:"weddingId"`
	CreatedAt time.Time `json:"createdAt"`
}
