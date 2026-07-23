package repository

import (
	"time"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type TokenRepo struct{ db *gorm.DB }

func NewTokenRepo(db *gorm.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Save(token *models.RefreshToken) error {
	return r.db.Create(token).Error
}

func (r *TokenRepo) FindByToken(token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.db.Where("token = ? AND expires_at > ?", token, time.Now()).First(&t).Error
	return &t, err
}

func (r *TokenRepo) DeleteByToken(token string) error {
	return r.db.Where("token = ?", token).Delete(&models.RefreshToken{}).Error
}

func (r *TokenRepo) DeleteExpired() error {
	return r.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}
