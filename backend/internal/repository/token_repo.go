package repository

import (
	"context"
	"errors"
	"fmt"
	"time"
	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TokenRepo struct{ db *gorm.DB }

func NewTokenRepo(db *gorm.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Save(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// ErrTokenClaimed reports that another caller rotated the token first.
var ErrTokenClaimed = errors.New("refresh token already rotated")

// lockAdmin takes a row lock on the token owner. Rotate holds it shared and
// DeleteByAdminID exclusive, so a revoke waits for an in-flight rotation to
// commit and then deletes its new token too. Without it, the DELETE's snapshot
// misses the uncommitted replacement row and that token survives the revoke.
func lockAdmin(tx *gorm.DB, adminID uuid.UUID, strength string) error {
	if tx.Name() != "postgres" {
		return nil
	}
	var ids []uuid.UUID
	if err := tx.Model(&models.AdminUser{}).Clauses(clause.Locking{Strength: strength}).
		Where("id = ?", adminID).Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("lock admin: %w", err)
	}
	return nil
}

// Rotate consumes token and stores its replacement in one transaction. The
// conditional update lets exactly one concurrent caller win; the loser gets
// ErrTokenClaimed and falls back to the replay path. Any failure rolls back, so
// a token is never consumed without its successor being stored.
func (r *TokenRepo) Rotate(ctx context.Context, token string, replacement *models.RefreshToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdmin(tx, replacement.AdminID, "SHARE"); err != nil {
			return err
		}
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
// refresh token, so Refresh re-mints a scoped access token. Other devices of
// the same user keep their own scope. Reports whether a row matched.
func (r *TokenRepo) UpdateWeddingScope(ctx context.Context, adminID uuid.UUID, token string, weddingID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("admin_id = ? AND token = ? AND expires_at > ?", adminID, token, time.Now()).
		Update("wedding_id", weddingID)
	return res.RowsAffected > 0, res.Error
}

// DeleteExpired removes refresh tokens that expired before cutoff.
func (r *TokenRepo) DeleteExpired(ctx context.Context, cutoff time.Time) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", cutoff).Delete(&models.RefreshToken{}).Error
}

// DeleteByAdminID deletes all refresh tokens for the given admin user.
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdmin(tx, adminID, "UPDATE"); err != nil {
			return err
		}
		return tx.Where("admin_id = ?", adminID).Delete(&models.RefreshToken{}).Error
	})
}
