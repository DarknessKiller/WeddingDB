package repository

import (
	"context"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WeddingRepo struct{ db *gorm.DB }

func NewWeddingRepo(db *gorm.DB) *WeddingRepo {
	return &WeddingRepo{db: db}
}

func (r *WeddingRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.WeddingEvent, error) {
	var w models.WeddingEvent
	err := r.db.WithContext(ctx).First(&w, id).Error
	return &w, err
}

func (r *WeddingRepo) List(ctx context.Context) ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.WithContext(ctx).Find(&weddings).Error
	return weddings, err
}

func (r *WeddingRepo) Create(ctx context.Context, w *models.WeddingEvent) error {
	return r.db.WithContext(ctx).Create(w).Error
}

// UpdateFields writes only the named columns, so a save of one form does not
// revert edits another device made to columns it did not load.
func (r *WeddingRepo) UpdateFields(ctx context.Context, w *models.WeddingEvent, columns ...string) error {
	if len(columns) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&models.WeddingEvent{}).
		Where("id = ?", w.ID).
		Select(columns).
		Updates(w).Error
}

func (r *WeddingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Cascade: remove guests, tables, hall elements, and user associations
		if err := tx.Where("wedding_id = ?", id).Delete(&models.GuestRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("wedding_id = ?", id).Delete(&models.BanquetTable{}).Error; err != nil {
			return err
		}
		if err := tx.Where("wedding_id = ?", id).Delete(&models.HallElement{}).Error; err != nil {
			return err
		}
		if err := tx.Where("wedding_id = ?", id).Delete(&models.UserWedding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("wedding_id = ?", id).Delete(&models.AppliedMutation{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.WeddingEvent{}, id).Error
	})
}
