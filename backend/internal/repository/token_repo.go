package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TokenRepo struct{ db *gorm.DB }

func NewTokenRepo(db *gorm.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

// HashToken returns the stored form of a refresh token, its SHA-256 hex, so a
// leaked table holds no usable tokens. Every lookup hashes its input first.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// hashed returns a copy of token that stores the hash, leaving the caller's
// plaintext in place for the response.
func hashed(token *models.RefreshToken) *models.RefreshToken {
	row := *token
	row.Token = HashToken(token.Token)
	return &row
}

func (r *TokenRepo) Save(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(hashed(token)).Error
}

// ErrTokenClaimed reports that another caller rotated the token first.
var ErrTokenClaimed = errors.New("refresh token already rotated")

// Rotate consumes token and stores its replacement in one transaction. The
// conditional update lets exactly one concurrent caller win; the loser gets
// ErrTokenClaimed and falls back to the replay path. Any failure rolls back, so
// a token is never consumed without its successor being stored.
func (r *TokenRepo) Rotate(ctx context.Context, token string, replacement *models.RefreshToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.RefreshToken{}).
			Where("token = ? AND rotated_at IS NULL", HashToken(token)).
			Updates(map[string]any{"rotated_at": time.Now(), "replaced_by": HashToken(replacement.Token)})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTokenClaimed
		}
		return tx.Create(hashed(replacement)).Error
	})
}

func (r *TokenRepo) FindByToken(ctx context.Context, token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.db.WithContext(ctx).Where("token = ? AND expires_at > ?", HashToken(token), time.Now()).First(&t).Error
	return &t, err
}

func (r *TokenRepo) DeleteByToken(ctx context.Context, token string) (int64, error) {
	res := r.db.WithContext(ctx).Where("token = ?", HashToken(token)).Delete(&models.RefreshToken{})
	return res.RowsAffected, res.Error
}

// UpdateWeddingScope persists the selected wedding onto one session's live
// refresh token, so Refresh re-mints a scoped access token instead of dropping
// back to an unscoped (nil) wedding. Other sessions keep their own scope.
func (r *TokenRepo) UpdateWeddingScope(ctx context.Context, adminID, sessionID, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("id = ? AND admin_id = ? AND expires_at > ?", sessionID, adminID, time.Now()).
		Update("wedding_id", weddingID).Error
}

func (r *TokenRepo) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}

// DeleteByAdminID deletes all refresh tokens for the given admin user.
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("admin_id = ?", adminID).Delete(&models.RefreshToken{}).Error
}
