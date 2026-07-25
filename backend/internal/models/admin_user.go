package models

import (
	"time"

	"github.com/google/uuid"
)

type AdminUser struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email               string         `gorm:"size:255;not null" json:"email"`
	Password            string         `gorm:"size:255;not null" json:"-"`
	Name                string         `gorm:"size:255" json:"name"`
	Role                string         `gorm:"size:20;not null" json:"role"`
	ForcePasswordChange bool           `gorm:"default:false" json:"-"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	Weddings            []WeddingEvent `gorm:"many2many:user_weddings;" json:"weddings,omitempty"`
}
