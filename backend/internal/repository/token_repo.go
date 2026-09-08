package repository

import (
	"context"
	"time"
	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TokenRepo struct{ db *gorm.DB }

func NewTokenRepo(db *gorm.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Save(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *TokenRepo) FindByToken(ctx context.Context, token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.db.WithContext(ctx).Where("token = ? AND expires_at > ?", token, time.Now()).First(&t).Error
	return &t, err
}

func (r *TokenRepo) DeleteByToken(ctx context.Context, token string) (int64, error) {
	res := r.db.WithContext(ctx).Where("token = ?", token).Delete(&models.RefreshToken{})
	return res.RowsAffected, res.Error
}

// UpdateWeddingScope persists the selected wedding onto all live refresh
// tokens for the given admin, so Refresh re-mints a scoped access token
// instead of dropping back to an unscoped (nil) wedding.
func (r *TokenRepo) UpdateWeddingScope(ctx context.Context, adminID uuid.UUID, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("admin_id = ? AND expires_at > ?", adminID, time.Now()).
		Update("wedding_id", weddingID).Error
}

func (r *TokenRepo) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}

// DeleteByAdminID deletes all refresh tokens for the given admin user.
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("admin_id = ?", adminID).Delete(&models.RefreshToken{}).Error
}
