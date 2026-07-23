package repository

import (
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type WeddingRepo struct{ db *gorm.DB }

func NewWeddingRepo(db *gorm.DB) *WeddingRepo {
	return &WeddingRepo{db: db}
}

func (r *WeddingRepo) FindByID(id uint) (*models.WeddingEvent, error) {
	var w models.WeddingEvent
	err := r.db.First(&w, id).Error
	return &w, err
}

func (r *WeddingRepo) List() ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.Find(&weddings).Error
	return weddings, err
}

func (r *WeddingRepo) Create(w *models.WeddingEvent) error {
	return r.db.Create(w).Error
}

func (r *WeddingRepo) Update(w *models.WeddingEvent) error {
	return r.db.Save(w).Error
}

func (r *WeddingRepo) Delete(id uint) error {
	return r.db.Delete(&models.WeddingEvent{}, id).Error
}
