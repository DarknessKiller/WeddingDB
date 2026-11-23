package repository

import (
	"context"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// CreateWithOwner inserts the wedding and the creator's grant in one
// transaction, so a grant failure leaves no ownerless wedding behind.
func (r *WeddingRepo) CreateWithOwner(ctx context.Context, w *models.WeddingEvent, ownerID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(w).Error; err != nil {
			return err
		}
		if ownerID == uuid.Nil {
			return nil
		}
		return tx.Create(&models.UserWedding{ID: uuid.New(), UserID: ownerID, WeddingID: w.ID}).Error
	})
}

// Update writes the mutable columns only, under the row lock. Save would
// re-insert a deleted wedding: this report is 404 instead.
func (r *WeddingRepo) Update(ctx context.Context, w *models.WeddingEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.WeddingEvent{}).Where("id = ?", w.ID)
		if tx.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		// Read the row under the lock. A count would be invalid here: Postgres
		// rejects FOR UPDATE with an aggregate.
		var existing models.WeddingEvent
		if err := q.First(&existing).Error; err != nil {
			return err
		}
		return tx.Model(&models.WeddingEvent{}).Where("id = ?", w.ID).Updates(map[string]any{
			"name":                     w.Name,
			"date":                     w.Date,
			"venue_name":               w.VenueName,
			"venue_address":            w.VenueAddress,
			"kiosk_description":        w.KioskDescription,
			"kiosk_logo_url":           w.KioskLogoUrl,
			"kiosk_background_url":     w.KioskBackgroundUrl,
			"kiosk_background_blur":    w.KioskBackgroundBlur,
			"kiosk_background_size":    w.KioskBackgroundSize,
			"kiosk_background_pos_x":   w.KioskBackgroundPosX,
			"kiosk_background_pos_y":   w.KioskBackgroundPosY,
			"kiosk_logo_size":          w.KioskLogoSize,
			"kiosk_logo_pos_x":         w.KioskLogoPosX,
			"kiosk_logo_pos_y":         w.KioskLogoPosY,
			"show_seat_numbers":        w.ShowSeatNumbers,
			"kiosk_idle_reset_seconds": w.KioskIdleResetSeconds,
			"hall_width":               w.HallWidth,
			"hall_height":              w.HallHeight,
		}).Error
	})
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
		return tx.Delete(&models.WeddingEvent{}, id).Error
	})
}
