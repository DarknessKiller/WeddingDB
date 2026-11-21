package repository

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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

// HashToken is the stored form of a refresh token. The database never holds
// the secret itself, so a leaked dump or backup cannot resume a session.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// hashedTokenLen is len(HashToken(x)). Rows of any other length predate
// hashing and still hold the raw secret.
var hashedTokenLen = base64.RawURLEncoding.EncodedLen(sha256.Size)

// hashed returns a copy of token with its secret replaced by its hash, so the
// caller keeps the raw value to hand to the client.
func hashed(token *models.RefreshToken) *models.RefreshToken {
	stored := *token
	stored.Token = HashToken(token.Token)
	return &stored
}

// Save stores token. token.Token is the raw secret; only its hash is written.
func (r *TokenRepo) Save(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(hashed(token)).Error
}

// DeleteLegacyPlaintext removes refresh tokens stored before hashing. Their
// owners log in again once.
func (r *TokenRepo) DeleteLegacyPlaintext(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("LENGTH(token) <> ?", hashedTokenLen).Delete(&models.RefreshToken{}).Error
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

// FindByToken looks up a live token by its raw secret. The returned row's
// Token and ReplacedBy hold hashes, never raw secrets.
func (r *TokenRepo) FindByToken(ctx context.Context, token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.db.WithContext(ctx).Where("token = ? AND expires_at > ?", HashToken(token), time.Now()).First(&t).Error
	return &t, err
}

func (r *TokenRepo) DeleteByToken(ctx context.Context, token string) (int64, error) {
	res := r.db.WithContext(ctx).Where("token = ?", HashToken(token)).Delete(&models.RefreshToken{})
	return res.RowsAffected, res.Error
}

// UpdateWeddingScope persists the selected wedding onto the caller's own live
// refresh token, so Refresh re-mints a scoped access token. Other devices of
// the same user keep their own scope. Reports whether a row matched.
func (r *TokenRepo) UpdateWeddingScope(ctx context.Context, adminID uuid.UUID, token string, weddingID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("admin_id = ? AND token = ? AND rotated_at IS NULL AND expires_at > ?", adminID, HashToken(token), time.Now()).
		Update("wedding_id", weddingID)
	return res.RowsAffected > 0, res.Error
}

// DeleteExpired removes refresh tokens that expired before cutoff.
func (r *TokenRepo) DeleteExpired(ctx context.Context, cutoff time.Time) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", cutoff).Delete(&models.RefreshToken{}).Error
}

// DeleteByAdminID deletes all refresh tokens for the given admin user and,
// under the same admin row lock, sets their token version to
// max(token_version, minVersion)+1. minVersion carries a cached version that
// may be ahead of the row (versions once lived only in Redis).
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID, minVersion int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdmin(tx, adminID, "UPDATE"); err != nil {
			return err
		}
		if err := tx.Where("admin_id = ?", adminID).Delete(&models.RefreshToken{}).Error; err != nil {
			return fmt.Errorf("delete refresh tokens: %w", err)
		}
		if err := tx.Model(&models.AdminUser{}).Where("id = ?", adminID).
			UpdateColumn("token_version", gorm.Expr("CASE WHEN token_version < ? THEN ? ELSE token_version END + 1", minVersion, minVersion)).Error; err != nil {
			return fmt.Errorf("bump token version: %w", err)
		}
		return nil
	})
}
