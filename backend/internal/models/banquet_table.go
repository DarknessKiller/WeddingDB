package models

import "time"

type BanquetTable struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	WeddingID uint      `gorm:"index;not null" json:"-"`
	Name      string    `gorm:"size:100;not null" json:"n"`
	Capacity  int       `gorm:"not null" json:"cap"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	IsVip     bool      `json:"v"`
	Zone      string    `gorm:"size:20" json:"z"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
}
