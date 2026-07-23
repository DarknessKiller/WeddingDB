package models

import "time"

type WeddingEvent struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Name      string    `gorm:"size:255;not null" json:"n"`
	Date      time.Time `json:"d"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
