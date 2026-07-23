package services

import (
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type WeddingService struct {
	weddingRepo *repository.WeddingRepo
}

func NewWeddingService(weddingRepo *repository.WeddingRepo) *WeddingService {
	return &WeddingService{weddingRepo: weddingRepo}
}

func (s *WeddingService) List() ([]models.WeddingEvent, error) {
	return s.weddingRepo.List()
}

func (s *WeddingService) Get(id uint) (*models.WeddingEvent, error) {
	return s.weddingRepo.FindByID(id)
}

func (s *WeddingService) Create(w *models.WeddingEvent) error {
	return s.weddingRepo.Create(w)
}

func (s *WeddingService) Update(w *models.WeddingEvent) error {
	return s.weddingRepo.Update(w)
}

func (s *WeddingService) Delete(id uint) error {
	return s.weddingRepo.Delete(id)
}
