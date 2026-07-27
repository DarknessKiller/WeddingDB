package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/utils"
)

var ElementTypes = []string{"stage", "dj_counter", "entrance", "tv", "walkway", "box"}

func ValidElementType(t string) bool {
	for _, v := range ElementTypes {
		if v == t {
			return true
		}
	}
	return false
}

type HallElement struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID  uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name       string    `gorm:"size:255;not null" json:"name"`
	Type       string    `gorm:"size:20;not null" json:"type"`
	X          float64   `gorm:"not null;default:0" json:"x"`
	Y          float64   `gorm:"not null;default:0" json:"y"`
	Degree     float64   `gorm:"not null;default:0" json:"degree"`
	Width      float64   `gorm:"not null;default:0" json:"width"`
	Height     float64   `gorm:"not null;default:0" json:"height"`
	Color      string    `gorm:"size:20" json:"color"`
	TextColor  string    `gorm:"size:20" json:"textColor"`
	StrokeColor string   `gorm:"size:20" json:"strokeColor"`
	Opacity    float64   `gorm:"not null;default:1" json:"opacity"`
	ZIndex     int       `gorm:"not null;default:0" json:"zIndex"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (e HallElement) MarshalJSON() ([]byte, error) {
	type Alias HallElement
	return json.Marshal(&struct {
		ID        string `json:"id"`
		WeddingID string `json:"weddingId"`
		Alias
	}{
		ID:        utils.EncodeUUID(e.ID),
		WeddingID: utils.EncodeUUID(e.WeddingID),
		Alias:     (Alias)(e),
	})
}
