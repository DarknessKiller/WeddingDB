package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type AdminRepo struct{ db *gorm.DB }

func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

func (r *AdminRepo) FindByEmail(email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.Where("email = ?", email).First(&admin).Error
	return &admin, err
}

func (r *AdminRepo) FindByID(id uuid.UUID) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.First(&admin, id).Error
	return &admin, err
}

func (r *AdminRepo) Create(admin *models.AdminUser) error {
	return r.db.Create(admin).Error
}

func (r *AdminRepo) List() ([]models.AdminUser, error) {
	var admins []models.AdminUser
	err := r.db.Find(&admins).Error
	return admins, err
}

func (r *AdminRepo) Update(admin *models.AdminUser) error {
	return r.db.Save(admin).Error
}

// CountByRole returns the number of admins with the given role.
func (r *AdminRepo) CountByRole(role string) (int64, error) {
	var count int64
	err := r.db.Model(&models.AdminUser{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

func (r *AdminRepo) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.AdminUser{}, id).Error
}

// GetUserWeddings returns all weddings a user has access to.
func (r *AdminRepo) GetUserWeddings(userID uuid.UUID) ([]models.WeddingEvent, error) {
	var weddings []models.WeddingEvent
	err := r.db.Joins("JOIN user_weddings ON user_weddings.wedding_id = wedding_events.id").
		Where("user_weddings.user_id = ?", userID).
		Find(&weddings).Error
	return weddings, err
}

// SetUserWeddings replaces a user's wedding associations.
func (r *AdminRepo) SetUserWeddings(userID uuid.UUID, weddingIDs []uuid.UUID) error {
	// Delete existing
	r.db.Where("user_id = ?", userID).Delete(&models.UserWedding{})
	// Insert new
	for _, wid := range weddingIDs {
		uw := models.UserWedding{
			ID:        uuid.New(),
			UserID:    userID,
			WeddingID: wid,
		}
		if err := r.db.Create(&uw).Error; err != nil {
			return err
		}
	}
	return nil
}

// HasWeddingAccess checks if a user has access to a specific wedding.
func (r *AdminRepo) HasWeddingAccess(userID, weddingID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&models.UserWedding{}).
		Where("user_id = ? AND wedding_id = ?", userID, weddingID).
		Count(&count).Error
	return count > 0, err
}

// AddUserWedding adds a wedding association if it doesn't already exist.
func (r *AdminRepo) AddUserWedding(userID, weddingID uuid.UUID) error {
	exists, _ := r.HasWeddingAccess(userID, weddingID)
	if exists {
		return nil
	}
	uw := models.UserWedding{
		ID:        uuid.New(),
		UserID:    userID,
		WeddingID: weddingID,
	}
	return r.db.Create(&uw).Error
}
