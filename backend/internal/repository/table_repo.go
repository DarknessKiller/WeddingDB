package repository

import (
	"context"
	"fmt"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

// CheckCapacity refuses a capacity below the last occupied seat, so shrinking a
// table cannot strand a guest who is already seated past it. The caller holds
// the table row lock inside its transaction.
func (r *TableRepo) CheckCapacity(ctx context.Context, weddingID, tableID uuid.UUID, capacity int) error {
	row := r.db.WithContext(ctx).Model(&models.GuestRecord{}).
		Select("COALESCE(MAX(seat_num + pax - 1), 0)").
		Where("wedding_id = ? AND table_id = ? AND seat_num IS NOT NULL", weddingID, tableID).
		Row()
	var highest int
	if err := row.Scan(&highest); err != nil {
		return err
	}
	if highest > capacity {
		return fmt.Errorf("%w: seats up to %d are taken", ErrCapacityBelowSeated, highest)
	}
	return nil
}

func (r *TableRepo) Update(ctx context.Context, t *models.BanquetTable) error {
	return r.db.WithContext(ctx).Model(t).Where("id = ? AND wedding_id = ?", t.ID, t.WeddingID).Updates(map[string]interface{}{
		"name":     t.Name,
		"capacity": t.Capacity,
		"x":        t.X,
		"y":        t.Y,
		"degree":   t.Degree,
		"is_vip":   t.IsVip,
	}).Error
}

func (r *TableRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.BanquetTable{}).Error
}
