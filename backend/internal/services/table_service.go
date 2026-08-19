package services

import (
	"context"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type TableService struct {
	tableRepo *repository.TableRepo
	guestRepo *repository.GuestRepo
}

func NewTableService(tableRepo *repository.TableRepo, guestRepo *repository.GuestRepo) *TableService {
	return &TableService{tableRepo: tableRepo, guestRepo: guestRepo}
}

func (s *TableService) List(ctx context.Context, weddingID uuid.UUID) ([]models.BanquetTable, error) {
	return s.tableRepo.ListByWedding(ctx, weddingID)
}

func (s *TableService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.BanquetTable, error) {
	return s.tableRepo.FindByID(ctx, id, weddingID)
}

func (s *TableService) Create(ctx context.Context, t *models.BanquetTable) error {
	return s.tableRepo.Create(ctx, t)
}

func (s *TableService) Update(ctx context.Context, t *models.BanquetTable) error {
	return s.tableRepo.Update(ctx, t)
}

func (s *TableService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Unassign guests from this table before deleting
	if err := s.guestRepo.UnassignByTable(ctx, weddingID, id); err != nil {
		return err
	}
	return s.tableRepo.Delete(ctx, id, weddingID)
}
