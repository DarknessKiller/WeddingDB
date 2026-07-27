package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/utils"
)

type BanquetTable struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Capacity  int       `gorm:"not null" json:"capacity"`
	X         float64   `gorm:"not null;default:0" json:"x"`
	Y         float64   `gorm:"not null;default:0" json:"y"`
	Degree    float64   `gorm:"not null;default:0" json:"degree"`
	IsVip     bool      `json:"isVip"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (t BanquetTable) MarshalJSON() ([]byte, error) {
	type Alias BanquetTable
	return json.Marshal(&struct {
		ID        string `json:"id"`
		WeddingID string `json:"weddingId"`
		Alias
	}{
		ID:        utils.EncodeUUID(t.ID),
		WeddingID: utils.EncodeUUID(t.WeddingID),
		Alias:     (Alias)(t),
	})
}
