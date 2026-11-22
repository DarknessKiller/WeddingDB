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

// Rotate consumes token and stores its replacement in one transaction. The
// conditional update lets exactly one concurrent caller win; the loser gets
// ErrTokenClaimed and falls back to the replay path. Any failure rolls back, so
// a token is never consumed without its successor being stored.
func (r *TokenRepo) Rotate(ctx context.Context, token string, replacement *models.RefreshToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.RefreshToken{}).
			Where("token = ? AND rotated_at IS NULL", token).
			Updates(map[string]any{"rotated_at": time.Now(), "replaced_by": replacement.Token})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTokenClaimed
		}
		return tx.Create(replacement).Error
	})
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

// UpdateWeddingScope persists the selected wedding onto the caller's own live
// refresh token, so Refresh re-mints a scoped access token instead of dropping
// back to an unscoped (nil) wedding. Other sessions of the same user keep theirs.
// A token that matches no live row is reported as not found: silently returning
// success would hand back a token whose scope was never persisted.
func (r *TokenRepo) UpdateWeddingScope(ctx context.Context, adminID uuid.UUID, token string, weddingID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("admin_id = ? AND token = ? AND expires_at > ?", adminID, token, time.Now()).
		Update("wedding_id", weddingID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteByAdminID deletes all refresh tokens for the given admin user.
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("admin_id = ?", adminID).Delete(&models.RefreshToken{}).Error
}
