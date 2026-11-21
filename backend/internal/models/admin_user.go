package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminUser struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email               string    `gorm:"size:255;not null" json:"email"`
	Password            string    `gorm:"size:255;not null" json:"-"`
	Name                string    `gorm:"size:255" json:"name"`
	Role                string    `gorm:"size:20;not null" json:"role"`
	ForcePasswordChange bool      `gorm:"default:false" json:"-"`
	// TokenVersion is the source of truth for access-token revocation; Redis caches it.
	TokenVersion int            `gorm:"not null;default:0" json:"-"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	Weddings     []WeddingEvent `gorm:"many2many:user_weddings;" json:"weddings,omitempty"`
}

// BeforeSave stores emails in one case so login and uniqueness ignore case.
func (a *AdminUser) BeforeSave(*gorm.DB) error {
	a.Email = strings.ToLower(strings.TrimSpace(a.Email))
	return nil
}
