package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type TableRepo struct{ db *gorm.DB }

func NewTableRepo(db *gorm.DB) *TableRepo {
	return &TableRepo{db: db}
}

func (r *TableRepo) ListByWedding(weddingID uuid.UUID) ([]models.BanquetTable, error) {
	var tables []models.BanquetTable
	err := r.db.Where("wedding_id = ?", weddingID).Find(&tables).Error
	return tables, err
}

func (r *TableRepo) FindByID(id, weddingID uuid.UUID) (*models.BanquetTable, error) {
	var t models.BanquetTable
	err := r.db.Where("id = ? AND wedding_id = ?", id, weddingID).First(&t).Error
	return &t, err
}

func (r *TableRepo) Create(t *models.BanquetTable) error {
	return r.db.Create(t).Error
}

func (r *TableRepo) Update(t *models.BanquetTable) error {
	return r.db.Save(t).Error
}

func (r *TableRepo) Delete(id, weddingID uuid.UUID) error {
	return r.db.Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.BanquetTable{}).Error
}
