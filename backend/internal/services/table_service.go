package services

import (
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

func (s *TableService) List(weddingID uuid.UUID) ([]models.BanquetTable, error) {
	return s.tableRepo.ListByWedding(weddingID)
}

func (s *TableService) Get(id, weddingID uuid.UUID) (*models.BanquetTable, error) {
	return s.tableRepo.FindByID(id, weddingID)
}

func (s *TableService) Create(t *models.BanquetTable) error {
	return s.tableRepo.Create(t)
}

func (s *TableService) Update(t *models.BanquetTable) error {
	return s.tableRepo.Update(t)
}

func (s *TableService) Delete(id, weddingID uuid.UUID) error {
	return s.tableRepo.Delete(id, weddingID)
}
