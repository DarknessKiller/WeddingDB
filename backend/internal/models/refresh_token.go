package models

import "time"

type RefreshToken struct {
	ID        uint      `gorm:"primaryKey"`
	AdminID   uint      `gorm:"index;not null"`
	Token     string    `gorm:"size:255;uniqueIndex;not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}
