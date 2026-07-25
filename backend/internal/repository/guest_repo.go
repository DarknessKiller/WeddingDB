package repository

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type GuestRepo struct{ db *gorm.DB }

func NewGuestRepo(db *gorm.DB) *GuestRepo {
	return &GuestRepo{db: db}
}

func (r *GuestRepo) ListByWedding(weddingID uuid.UUID, offset, limit int) ([]models.GuestRecord, int64, error) {
	guests := make([]models.GuestRecord, 0)
	var total int64
	r.db.Model(&models.GuestRecord{}).Where("wedding_id = ?", weddingID).Count(&total)
	err := r.db.Where("wedding_id = ?", weddingID).Offset(offset).Limit(limit).Find(&guests).Error
	return guests, total, err
}

func (r *GuestRepo) FindByID(id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	var g models.GuestRecord
	err := r.db.Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

func (r *GuestRepo) SearchByWedding(weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	escaped := strings.ReplaceAll(query, "%", "\\%")
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	q := fmt.Sprintf("%%%s%%", escaped)
	err := r.db.Where("wedding_id = ? AND (name ILIKE ? OR phone ILIKE ? OR email ILIKE ?)",
		weddingID, q, q, q).Limit(20).Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) Create(g *models.GuestRecord) error {
	return r.db.Create(g).Error
}

func (r *GuestRepo) Update(g *models.GuestRecord) error {
	return r.db.Save(g).Error
}

func (r *GuestRepo) Delete(id, weddingID uuid.UUID) error {
	return r.db.Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.GuestRecord{}).Error
}

func (r *GuestRepo) TableOccupancy(weddingID uuid.UUID) ([]TableOccupancy, error) {
	type row struct {
		TableID uuid.UUID
		Pax     int
	}
	rows := make([]row, 0)
	err := r.db.Model(&models.GuestRecord{}).
		Select("table_id, SUM(pax) as pax").
		Where("wedding_id = ? AND table_id IS NOT NULL", weddingID).
		Group("table_id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]TableOccupancy, 0, len(rows))
	for _, row := range rows {
		result = append(result, TableOccupancy{TableID: row.TableID, Pax: row.Pax})
	}
	return result, nil
}

type TableOccupancy struct {
	TableID uuid.UUID `json:"TableID"`
	Pax     int       `json:"Pax"`
}
