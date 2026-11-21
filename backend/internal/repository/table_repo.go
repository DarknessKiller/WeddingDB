package repository

import (
	"context"
	"errors"
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

func (r *TableRepo) Update(ctx context.Context, t *models.BanquetTable) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(t).Where("id = ? AND wedding_id = ?", t.ID, t.WeddingID).Updates(map[string]interface{}{
			"name":     t.Name,
			"capacity": t.Capacity,
			"x":        t.X,
			"y":        t.Y,
			"degree":   t.Degree,
			"is_vip":   t.IsVip,
		}).Error; err != nil {
			return err
		}
		return checkCapacityCoversSeats(tx, t.WeddingID, t.ID, t.Capacity)
	})
}

// ErrCapacityBelowSeated is returned when a capacity change would leave seated
// guests beyond the table's last seat.
var ErrCapacityBelowSeated = errors.New("capacity is below seated guests")

// checkCapacityCoversSeats fails when capacity is below the last seat a seated
// guest of the table occupies. Call it after writing the table row in the same
// transaction: the write holds the row lock seat assignments take, so no seat
// can be assigned between the check and the commit.
func checkCapacityCoversSeats(tx *gorm.DB, weddingID, tableID uuid.UUID, capacity int) error {
	var lastSeat int
	if err := tx.Model(&models.GuestRecord{}).
		Where("wedding_id = ? AND table_id = ? AND seat_num IS NOT NULL", weddingID, tableID).
		Select("COALESCE(MAX(seat_num + pax - 1), 0)").Row().Scan(&lastSeat); err != nil {
		return fmt.Errorf("find last seated seat: %w", err)
	}
	if capacity >= lastSeat {
		return nil
	}
	var name string
	if err := tx.Model(&models.BanquetTable{}).Where("id = ?", tableID).Select("name").Row().Scan(&name); err != nil {
		return fmt.Errorf("find table name: %w", err)
	}
	return fmt.Errorf("%w: table \"%s\" has guests seated up to seat %d, capacity %d is too small", ErrCapacityBelowSeated, name, lastSeat, capacity)
}

func (r *TableRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.BanquetTable{}).Error
}
