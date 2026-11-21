package repository

import (
	"context"
	"errors"

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

// Create stores a new wedding together with the default hall layout, so the
// seating map has a stage and entrance from the start.
// Create stores the wedding, its default layout, and (when ownerID is set)
// the owner's access link in one transaction.
func (r *WeddingRepo) Create(ctx context.Context, w *models.WeddingEvent, ownerID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(w).Error; err != nil {
			return err
		}
		if err := tx.Create(models.DefaultElements(w.ID)).Error; err != nil {
			return err
		}
		if ownerID == uuid.Nil {
			return nil
		}
		return tx.Create(&models.UserWedding{ID: uuid.New(), UserID: ownerID, WeddingID: w.ID}).Error
	})
}

// UpdateBasic writes only the fields the basic edit form owns. Replacing the
// whole row would overwrite a kiosk-settings save made from a stale read.
func (r *WeddingRepo) UpdateBasic(ctx context.Context, w *models.WeddingEvent) error {
	return r.db.WithContext(ctx).Model(w).Select("name", "date").Updates(w).Error
}

// UpdateKiosk writes only the kiosk presentation columns. includeName is set
// only when the payload carried a name, so an unrelated stale copy cannot
// overwrite a basic edit.
func (r *WeddingRepo) UpdateKiosk(ctx context.Context, w *models.WeddingEvent, includeName bool) error {
	cols := []string{
		"venue_name", "venue_address", "kiosk_description",
		"kiosk_logo_url", "kiosk_background_url", "kiosk_background_blur",
		"kiosk_background_size", "kiosk_background_pos_x", "kiosk_background_pos_y",
		"kiosk_logo_size", "kiosk_logo_pos_x", "kiosk_logo_pos_y",
		"show_seat_numbers", "kiosk_idle_reset_seconds",
	}
	if includeName {
		cols = append(cols, "name")
	}
	return r.db.WithContext(ctx).Model(w).Select(cols).Updates(w).Error
}

func (r *WeddingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the wedding row first, the way every guest, table and layout
		// write does: a write racing this delete then waits and fails on the
		// missing row instead of inserting an orphan after the cascade.
		if err := lockWedding(tx, id); errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // already gone: delete stays idempotent
		} else if err != nil {
			return err
		}
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
		return tx.Delete(&models.WeddingEvent{}, id).Error
	})
}
