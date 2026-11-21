package services

import (
	"context"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
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

func (s *WeddingService) Create(ctx context.Context, w *models.WeddingEvent, ownerID uuid.UUID) error {
	return s.weddingRepo.Create(ctx, w, ownerID)
}

func (s *WeddingService) UpdateBasic(ctx context.Context, w *models.WeddingEvent) error {
	return s.weddingRepo.UpdateBasic(ctx, w)
}

func (s *WeddingService) UpdateKiosk(ctx context.Context, w *models.WeddingEvent, includeName bool) error {
	return s.weddingRepo.UpdateKiosk(ctx, w, includeName)
}

func (s *WeddingService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.weddingRepo.Delete(ctx, id)
}
