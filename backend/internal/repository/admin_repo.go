package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminRepo struct{ db *gorm.DB }

func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

func (r *AdminRepo) FindByEmail(ctx context.Context, email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(email))).First(&admin).Error
	return &admin, err
}

func (r *AdminRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).First(&admin, id).Error
	return &admin, err
}

// TokenVersion returns the user's persisted token version.
func (r *AdminRepo) TokenVersion(ctx context.Context, id uuid.UUID) (int, error) {
	var admin models.AdminUser
	if err := r.db.WithContext(ctx).Select("token_version").First(&admin, id).Error; err != nil {
		return 0, fmt.Errorf("load token version: %w", err)
	}
	return admin.TokenVersion, nil
}

func (r *AdminRepo) Create(ctx context.Context, admin *models.AdminUser) error {
	return r.db.WithContext(ctx).Create(admin).Error
}

func (r *AdminRepo) List(ctx context.Context) ([]models.AdminUser, error) {
	var admins []models.AdminUser
	err := r.db.WithContext(ctx).Find(&admins).Error
	return admins, err
}

// SetPassword writes only the password hash and the force-change flag, so a
// concurrent role or profile change made from another request survives.
func (r *AdminRepo) SetPassword(ctx context.Context, id uuid.UUID, hash string, forceChange bool) error {
	res := r.db.WithContext(ctx).Model(&models.AdminUser{}).Where("id = ?", id).
		Updates(map[string]any{"password": hash, "force_password_change": forceChange})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountByRole returns the number of admins with the given role.
func (r *AdminRepo) CountByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.AdminUser{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

// ErrLastAdmin is returned when a write would leave no admin.
var ErrLastAdmin = errors.New("last admin")

// Delete removes a user, refusing with ErrLastAdmin to remove the last admin.
func (r *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.keepOneAdmin(ctx, id, func(tx *gorm.DB) error {
		return tx.Delete(&models.AdminUser{}, id).Error
	})
}

// UpdateRole sets a user's role, refusing with ErrLastAdmin to demote the last admin.
func (r *AdminRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	if role == "admin" {
		return r.db.WithContext(ctx).Model(&models.AdminUser{}).Where("id = ?", id).Update("role", role).Error
	}
	return r.keepOneAdmin(ctx, id, func(tx *gorm.DB) error {
		return tx.Model(&models.AdminUser{}).Where("id = ?", id).Update("role", role).Error
	})
}

// keepOneAdmin runs write after locking every admin row. A plain count (or a
// count subquery in the write) reads a snapshot, so two admins removing each
// other at once would both see two admins; the row locks serialise them.
func (r *AdminRepo) keepOneAdmin(ctx context.Context, id uuid.UUID, write func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admins []uuid.UUID
		if err := tx.Model(&models.AdminUser{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role = ?", "admin").Pluck("id", &admins).Error; err != nil {
			return fmt.Errorf("lock admins: %w", err)
		}
		if len(admins) <= 1 && slices.Contains(admins, id) {
			return ErrLastAdmin
		}
		return write(tx)
	})
}

// GetUserWeddings returns all weddings a user has access to.
func (r *AdminRepo) GetUserWeddings(ctx context.Context, userID uuid.UUID) ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.WithContext(ctx).Joins("JOIN user_weddings ON user_weddings.wedding_id = wedding_events.id").
		Where("user_weddings.user_id = ?", userID).
		Find(&weddings).Error
	return weddings, err
}

// SetUserWeddings replaces a user's wedding associations in a single transaction.
func (r *AdminRepo) SetUserWeddings(ctx context.Context, userID uuid.UUID, weddingIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return setUserWeddings(tx, userID, weddingIDs)
	})
}

// CreateWithWeddings creates a user and their wedding grants together, so a
// failed grant never leaves a half-made account behind.
func (r *AdminRepo) CreateWithWeddings(ctx context.Context, admin *models.AdminUser, weddingIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(admin).Error; err != nil {
			return err
		}
		// A new user has no grants yet, so there is nothing to delete first.
		return insertUserWeddings(tx, admin.ID, weddingIDs)
	})
}

func setUserWeddings(tx *gorm.DB, userID uuid.UUID, weddingIDs []uuid.UUID) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.UserWedding{}).Error; err != nil {
		return err
	}
	return insertUserWeddings(tx, userID, weddingIDs)
}

func insertUserWeddings(tx *gorm.DB, userID uuid.UUID, weddingIDs []uuid.UUID) error {
	for _, wid := range weddingIDs {
		uw := models.UserWedding{
			ID:        uuid.New(),
			UserID:    userID,
			WeddingID: wid,
		}
		if err := tx.Create(&uw).Error; err != nil {
			return err
		}
	}
	return nil
}

// HasWeddingAccess checks if a user has access to a specific wedding.
func (r *AdminRepo) HasWeddingAccess(ctx context.Context, userID, weddingID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.UserWedding{}).
		Where("user_id = ? AND wedding_id = ?", userID, weddingID).
		Count(&count).Error
	return count > 0, err
}

// AddUserWedding adds a wedding association if it doesn't already exist.
func (r *AdminRepo) AddUserWedding(ctx context.Context, userID, weddingID uuid.UUID) error {
	exists, err := r.HasWeddingAccess(ctx, userID, weddingID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	uw := models.UserWedding{
		ID:        uuid.New(),
		UserID:    userID,
		WeddingID: weddingID,
	}
	return r.db.WithContext(ctx).Create(&uw).Error
}
