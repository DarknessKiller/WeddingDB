package models

import "time"

type AdminUser struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	WeddingID *uint     `gorm:"index" json:"weddingId"`
	Email     string    `gorm:"size:255;not null" json:"email"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Name      string    `gorm:"size:255" json:"name"`
	Role      string    `gorm:"size:20;not null" json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
