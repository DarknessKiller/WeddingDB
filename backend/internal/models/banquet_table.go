package models

import "time"

type BanquetTable struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	WeddingID uint      `gorm:"index;not null" json:"weddingId"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Capacity  int       `gorm:"not null" json:"capacity"`
	Row       int       `gorm:"not null;default:1" json:"row"`
	Col       int       `gorm:"not null;default:1" json:"col"`
	IsVip     bool      `json:"isVip"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
