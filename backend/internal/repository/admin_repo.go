package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

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

func (r *AdminRepo) Update(ctx context.Context, admin *models.AdminUser) error {
	return r.db.WithContext(ctx).Save(admin).Error
}

// CountByRole returns the number of admins with the given role.
func (r *AdminRepo) CountByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.AdminUser{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

func (r *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.AdminUser{}, id).Error
}

// GetUserWeddings returns all weddings a user has access to.
func (r *AdminRepo) GetUserWeddings(ctx context.Context, userID uuid.UUID) ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.WithContext(ctx).Joins("JOIN user_weddings ON user_weddings.wedding_id = wedding_events.id").
		Where("user_weddings.user_id = ?", userID).
		Find(&weddings).Error
	return weddings, err
}

// SetUserWeddings replaces a user's wedding associations.
func (r *AdminRepo) SetUserWeddings(ctx context.Context, userID uuid.UUID, weddingIDs []uuid.UUID) error {
	// Delete existing
	r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.UserWedding{})
	// Insert new
	for _, wid := range weddingIDs {
		uw := models.UserWedding{
			ID:        uuid.New(),
			UserID:    userID,
			WeddingID: wid,
		}
		if err := r.db.WithContext(ctx).Create(&uw).Error; err != nil {
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
	exists, _ := r.HasWeddingAccess(ctx, userID, weddingID)
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
