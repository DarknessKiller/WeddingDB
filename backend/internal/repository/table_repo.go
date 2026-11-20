package repository

import (
	"context"
	"errors"
	"fmt"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrCapacityBelowSeats means a capacity cut would leave seated guests past the
// table's last seat.
var ErrCapacityBelowSeats = errors.New("capacity is below seats already assigned")

// CheckCapacity rejects a capacity smaller than the highest seat in use at the
// table. Call it inside the transaction that writes the capacity: it locks the
// wedding row, then the table row, the same order as every guest seat write.
func (r *TableRepo) CheckCapacity(ctx context.Context, weddingID, tableID uuid.UUID, capacity int) error {
	db := r.db.WithContext(ctx)
	if db.Name() == "postgres" {
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ?", weddingID).First(&models.WeddingEvent{}).Error; err != nil {
			return err
		}
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND wedding_id = ?", tableID, weddingID).First(&models.BanquetTable{}).Error; err != nil {
			return err
		}
	}
	var lastSeat int
	if err := db.Model(&models.GuestRecord{}).
		Where("wedding_id = ? AND table_id = ? AND seat_num IS NOT NULL", weddingID, tableID).
		Select("COALESCE(MAX(seat_num + pax - 1), 0)").Row().Scan(&lastSeat); err != nil {
		return err
	}
	if lastSeat > capacity {
		return fmt.Errorf("%w: seat %d is taken", ErrCapacityBelowSeats, lastSeat)
	}
	return nil
}

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
