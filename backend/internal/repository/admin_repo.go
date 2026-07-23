package repository

import (
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

func (r *AdminRepo) FindByID(id uint) (*models.AdminUser, error) {
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

func (r *AdminRepo) Delete(id uint) error {
	return r.db.Delete(&models.AdminUser{}, id).Error
}
