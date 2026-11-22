package services

import (
	"context"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
)

type LayoutService struct {
	layoutRepo *repository.LayoutRepo
}

func NewLayoutService(lr *repository.LayoutRepo) *LayoutService {
	return &LayoutService{layoutRepo: lr}
}

func (s *LayoutService) Elements(ctx context.Context, weddingID uuid.UUID) ([]models.HallElement, error) {
	return s.layoutRepo.ElementsByWedding(ctx, weddingID)
}

// Save replaces the layout in one transaction: the stored elements are read and
// reconciled under the same lock as the write, so a concurrent save cannot
// duplicate or drop an element.
func (s *LayoutService) Save(
	ctx context.Context,
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tables []models.TableUpdate,
	incoming []models.HallElement,
) error {
	return s.layoutRepo.SaveLayout(ctx, weddingID, hallWidth, hallHeight, tables, incoming)
}
