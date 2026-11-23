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

// CreateWithOwner creates the wedding and grants its creator access atomically.
func (s *WeddingService) CreateWithOwner(ctx context.Context, w *models.WeddingEvent, ownerID uuid.UUID) error {
	return s.weddingRepo.CreateWithOwner(ctx, w, ownerID)
}

// Update writes only the columns the caller carried.
func (s *WeddingService) Update(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	return s.weddingRepo.Update(ctx, id, fields)
}

func (s *WeddingService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.weddingRepo.Delete(ctx, id)
}
