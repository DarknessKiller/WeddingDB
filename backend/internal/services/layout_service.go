package services

import (
	"context"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
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

func (s *LayoutService) Save(
	ctx context.Context,
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tablePos map[uuid.UUID][3]float64,
	incoming []models.HallElement,
) error {
	existing, err := s.layoutRepo.ElementsByWedding(ctx, weddingID)
	if err != nil {
		return err
	}
	toCreate, toUpdate, toDelete := ReconcileElements(existing, incoming)
	return s.layoutRepo.SaveLayout(ctx, weddingID, hallWidth, hallHeight, tablePos, toCreate, toUpdate, toDelete)
}

// ReconcileElements diffs desired elements against stored ones (full-replace semantics).
// Incoming elements without an ID are creates; with an ID are updates;
// existing elements absent from incoming are deletes.
func ReconcileElements(existing, incoming []models.HallElement) (toCreate, toUpdate []models.HallElement, toDelete []uuid.UUID) {
	incomingIDs := make(map[uuid.UUID]bool, len(incoming))
	for _, e := range incoming {
		if e.ID == uuid.Nil {
			toCreate = append(toCreate, e)
		} else {
			toUpdate = append(toUpdate, e)
			incomingIDs[e.ID] = true
		}
	}
	for _, e := range existing {
		if !incomingIDs[e.ID] {
			toDelete = append(toDelete, e.ID)
		}
	}
	return toCreate, toUpdate, toDelete
}
