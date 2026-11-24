package repository

import (
	"context"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LayoutRepo struct{ db *gorm.DB }

func NewLayoutRepo(db *gorm.DB) *LayoutRepo { return &LayoutRepo{db: db} }

func (r *LayoutRepo) ElementsByWedding(ctx context.Context, weddingID uuid.UUID) ([]models.HallElement, error) {
	return elementsByWedding(r.db.WithContext(ctx), weddingID)
}

func elementsByWedding(db *gorm.DB, weddingID uuid.UUID) ([]models.HallElement, error) {
	els := make([]models.HallElement, 0)
	err := db.Where("wedding_id = ?", weddingID).Order("z_index").Find(&els).Error
	return els, err
}

// SaveLayout writes the hall size, tables and the reconciled element set in one
// transaction. The stored elements are read inside it after the wedding row
// lock, so a concurrent save cannot reconcile against a stale list and leave
// duplicates behind.
func (r *LayoutRepo) SaveLayout(
	ctx context.Context,
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tables []models.TableUpdate,
	incoming []models.HallElement,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// One locked read: the probe is the lock, so a concurrent delete cannot
		// land between an existence check and the lock.
		var exists models.WeddingEvent
		q := tx.Select("id").Where("id = ?", weddingID)
		if tx.Name() == "postgres" {
			if err := setLockTimeout(tx); err != nil {
				return err
			}
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&exists).Error; err != nil {
			return err
		}
		existing, err := elementsByWedding(tx.WithContext(ctx), weddingID)
		if err != nil {
			return err
		}
		toCreate, toUpdate, toDelete := models.ReconcileElements(existing, incoming)
		if hallWidth > 0 && hallHeight > 0 {
			if err := tx.Model(&models.WeddingEvent{}).Where("id = ?", weddingID).
				Updates(map[string]any{"hall_width": hallWidth, "hall_height": hallHeight}).Error; err != nil {
				return err
			}
		}
		for _, t := range tables {
			if err := NewTableRepo(tx.WithContext(ctx)).CheckCapacity(ctx, weddingID, t.ID, t.Capacity); err != nil {
				return err
			}
			res := tx.Model(&models.BanquetTable{}).Where("id = ? AND wedding_id = ?", t.ID, weddingID).
				Updates(map[string]any{
					"x": t.X, "y": t.Y, "degree": t.Degree,
					"capacity": t.Capacity, "is_vip": t.IsVip,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrLayoutTableNotFound
			}
		}
		for i := range toCreate {
			toCreate[i].WeddingID = weddingID
			if err := tx.Create(&toCreate[i]).Error; err != nil {
				return err
			}
		}
		for i := range toUpdate {
			res := tx.Model(&models.HallElement{}).Where("id = ? AND wedding_id = ?", toUpdate[i].ID, weddingID).
				Updates(map[string]any{
					"type": toUpdate[i].Type, "x": toUpdate[i].X, "y": toUpdate[i].Y,
					"degree": toUpdate[i].Degree, "width": toUpdate[i].Width, "height": toUpdate[i].Height,
					"name": toUpdate[i].Name, "color": toUpdate[i].Color,
					"text_color": toUpdate[i].TextColor, "stroke_color": toUpdate[i].StrokeColor,
					"opacity": toUpdate[i].Opacity, "z_index": toUpdate[i].ZIndex,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrLayoutElementNotFound
			}
		}
		if len(toDelete) > 0 {
			if err := tx.Where("id IN ? AND wedding_id = ?", toDelete, weddingID).
				Delete(&models.HallElement{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// LockWedding takes the wedding row lock every guest, table and layout write
// shares. The row lock is Postgres-only; sqlite has one writer.
func LockWedding(tx *gorm.DB, weddingID uuid.UUID) error {
	if tx.Name() != "postgres" {
		return nil
	}
	if err := setLockTimeout(tx); err != nil {
		return err
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").Where("id = ?", weddingID).First(&models.WeddingEvent{}).Error
}

// setLockTimeout bounds every lock wait for the rest of tx, so a write queued
// behind a slow one fails fast (SQLSTATE 55P03) instead of holding a pool
// connection until the holder commits.
func setLockTimeout(tx *gorm.DB) error {
	return tx.Exec("SET LOCAL lock_timeout = '5s'").Error
}
