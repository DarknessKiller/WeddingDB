package repository

import (
	"context"
	"errors"
	"slices"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrLastAdmin is returned when a delete or demotion would leave no admin.
var ErrLastAdmin = errors.New("cannot remove the last admin")

// guardLastAdmin fails with ErrLastAdmin when id is an admin and the only
// one. It locks every admin row first (Postgres; sqlite has one writer), so
// two concurrent removals cannot both see two admins.
func guardLastAdmin(tx *gorm.DB, id uuid.UUID) error {
	q := tx.Model(&models.AdminUser{}).Where("role = ?", "admin")
	if tx.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var admins []uuid.UUID
	if err := q.Pluck("id", &admins).Error; err != nil {
		return err
	}
	if slices.Contains(admins, id) && len(admins) <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// UpdateRole sets the account's role, refusing to demote the last admin.
func (r *AdminRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if role != "admin" {
			if err := guardLastAdmin(tx, id); err != nil {
				return err
			}
		}
		return tx.Model(&models.AdminUser{}).Where("id = ?", id).Update("role", role).Error
	})
}

type AdminRepo struct{ db *gorm.DB }

func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

func (r *AdminRepo) FindByEmail(ctx context.Context, email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&admin).Error
	return &admin, err
}

func (r *AdminRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).First(&admin, id).Error
	return &admin, err
}

func (r *AdminRepo) Create(ctx context.Context, admin *models.AdminUser) error {
	return r.db.WithContext(ctx).Create(admin).Error
}

func (r *AdminRepo) List(ctx context.Context) ([]models.AdminUser, error) {
	var admins []models.AdminUser
	err := r.db.WithContext(ctx).Find(&admins).Error
	return admins, err
}

// UpdateFields writes only the given columns of one account, so a role change
// cannot write back a password hash read before a concurrent reset.
func (r *AdminRepo) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.AdminUser{}).Where("id = ?", id).Updates(fields).Error
}

// Delete removes the account and its wedding grants in one transaction,
// refusing to delete the last admin.
func (r *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := guardLastAdmin(tx, id); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.UserWedding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.AdminUser{}, id).Error
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
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserWedding{}).Error; err != nil {
			return err
		}
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
	})
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
