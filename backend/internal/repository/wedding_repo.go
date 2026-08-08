package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
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

func (r *WeddingRepo) Update(ctx context.Context, w *models.WeddingEvent) error {
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *WeddingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Cascade: remove guests, tables, hall elements, and user associations
		tx.Where("wedding_id = ?", id).Delete(&models.GuestRecord{})
		tx.Where("wedding_id = ?", id).Delete(&models.BanquetTable{})
		tx.Where("wedding_id = ?", id).Delete(&models.HallElement{})
		tx.Where("wedding_id = ?", id).Delete(&models.UserWedding{})
		return tx.Delete(&models.WeddingEvent{}, id).Error
	})
}
