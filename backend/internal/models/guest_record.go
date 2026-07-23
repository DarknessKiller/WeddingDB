package models

import "time"

type GuestRecord struct {
	ID          uint       `gorm:"primaryKey" json:"-"`
	WeddingID   uint       `gorm:"index;not null" json:"-"`
	Name        string     `gorm:"size:255;not null" json:"n"`
	Phone       string     `gorm:"size:50" json:"p"`
	Email       string     `gorm:"size:255" json:"e"`
	Pax         int        `gorm:"not null;default:1" json:"x"`
	TableID     *uint      `gorm:"index" json:"-"`
	SeatNum     *int       `json:"-"`
	RSVP        string     `gorm:"size:20;default:no_response" json:"r"`
	CheckedInAt *time.Time `json:"cia"`
	Notes       string     `gorm:"type:text" json:"nt"`
	Dietary     []string   `gorm:"type:text[]" json:"d"`
	IsVip       bool       `json:"v"`
	AngbaoAmt   *int       `json:"a"`
	GiftItem    *string    `gorm:"size:255" json:"g"`
	CreatedAt   time.Time  `json:"c"`
	UpdatedAt   time.Time  `json:"u"`
}
