package models

import "time"

type AdminUser struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	WeddingID *uint     `gorm:"index" json:"-"`
	Email     string    `gorm:"size:255;not null" json:"e"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Name      string    `gorm:"size:255" json:"n"`
	Role      string    `gorm:"size:20;not null" json:"rl"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
