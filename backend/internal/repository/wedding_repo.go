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

// Update writes only the columns the caller carried, under the row lock, so a
// save cannot revert a column another request changed in the meantime. Save
// would re-insert a deleted wedding: this report is 404 instead.
func (r *WeddingRepo) Update(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.WeddingEvent{}).Where("id = ?", id)
		if tx.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		// Read the row under the lock. A count would be invalid here: Postgres
		// rejects FOR UPDATE with an aggregate.
		var existing models.WeddingEvent
		if err := q.First(&existing).Error; err != nil {
			return err
		}
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&models.WeddingEvent{}).Where("id = ?", id).Updates(fields).Error
	})
}

func (r *WeddingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Take the wedding row lock before the cascade: a guest write that holds
		// it would otherwise insert rows after the sweep and leave orphans.
		if err := lockWeddingRow(tx, id); err != nil {
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
		res := tx.Delete(&models.WeddingEvent{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
