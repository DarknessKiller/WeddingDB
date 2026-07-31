package services

import (
	"context"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type WeddingService struct {
	weddingRepo *repository.WeddingRepo
}

func NewWeddingService(weddingRepo *repository.WeddingRepo) *WeddingService {
	return &WeddingService{weddingRepo: weddingRepo}
}

func (s *WeddingService) List(ctx context.Context) ([]models.WeddingEvent, error) {
	return s.weddingRepo.List(ctx)
}

func (s *WeddingService) Get(ctx context.Context, id uuid.UUID) (*models.WeddingEvent, error) {
	return s.weddingRepo.FindByID(ctx, id)
}

func (s *WeddingService) Create(ctx context.Context, w *models.WeddingEvent) error {
	return s.weddingRepo.Create(ctx, w)
}

func (s *WeddingService) Update(ctx context.Context, w *models.WeddingEvent) error {
	return s.weddingRepo.Update(ctx, w)
}

func (s *WeddingService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.weddingRepo.Delete(ctx, id)
}
