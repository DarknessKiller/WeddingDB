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

// Create stores a new wedding with the default hall layout.
func (s *WeddingService) Create(ctx context.Context, w *models.WeddingEvent) error {
	return s.weddingRepo.Create(ctx, w, models.DefaultElements)
}

func (s *WeddingService) UpdateFields(ctx context.Context, w *models.WeddingEvent, columns ...string) error {
	return s.weddingRepo.UpdateFields(ctx, w, columns...)
}

func (s *WeddingService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.weddingRepo.Delete(ctx, id)
}
