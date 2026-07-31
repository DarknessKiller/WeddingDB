package services

import (
	"context"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type TableService struct {
	tableRepo *repository.TableRepo
}

func NewTableService(tableRepo *repository.TableRepo) *TableService {
	return &TableService{tableRepo: tableRepo}
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
	return s.tableRepo.Delete(ctx, id, weddingID)
}
