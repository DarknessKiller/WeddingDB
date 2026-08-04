package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type TableRepo struct{ db *gorm.DB }

func NewTableRepo(db *gorm.DB) *TableRepo {
	return &TableRepo{db: db}
}

func (r *TableRepo) ListByWedding(ctx context.Context, weddingID uuid.UUID) ([]models.BanquetTable, error) {
	tables := make([]models.BanquetTable, 0)
	err := r.db.WithContext(ctx).
		Where("wedding_id = ?", weddingID).
		Order("is_vip DESC, name ASC, id ASC").
		Find(&tables).Error
	return tables, err
}

func (r *TableRepo) FindByID(ctx context.Context, id, weddingID uuid.UUID) (*models.BanquetTable, error) {
	var t models.BanquetTable
	err := r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).First(&t).Error
	return &t, err
}

func (r *TableRepo) Create(ctx context.Context, t *models.BanquetTable) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *TableRepo) Update(ctx context.Context, t *models.BanquetTable) error {
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *TableRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.BanquetTable{}).Error
}
