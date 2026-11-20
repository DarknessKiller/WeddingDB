package services

import (
	"context"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TableService struct {
	tableRepo *repository.TableRepo
	guestRepo *repository.GuestRepo
	sseHub    *SSEHub
	db        *gorm.DB
}

func NewTableService(tableRepo *repository.TableRepo, guestRepo *repository.GuestRepo, sseHub *SSEHub, db *gorm.DB) *TableService {
	return &TableService{tableRepo: tableRepo, guestRepo: guestRepo, sseHub: sseHub, db: db}
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
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewTableRepo(tx)
		if err := repo.CheckCapacity(ctx, t.WeddingID, t.ID, t.Capacity); err != nil {
			return err
		}
		return repo.Update(ctx, t)
	})
}

func (s *TableService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	var affected []models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Same lock order as every guest write: wedding row first, table row
		// second. A concurrent AssignSeat either commits before this delete
		// (and gets unassigned below) or blocks and then fails its table
		// lookup, so no guest can end up pointing at a deleted table.
		if _, err := guestCountLocked(tx, weddingID); err != nil {
			return err
		}
		if tx.Name() == "postgres" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
				Where("id = ? AND wedding_id = ?", id, weddingID).First(&models.BanquetTable{}).Error; err != nil {
				return err
			}
		}
		txGuestRepo := repository.NewGuestRepo(tx)
		var err error
		affected, err = txGuestRepo.FindByTable(ctx, weddingID, id)
		if err != nil {
			return err
		}
		if err := txGuestRepo.UnassignByTable(ctx, weddingID, id); err != nil {
			return err
		}
		return repository.NewTableRepo(tx).Delete(ctx, id, weddingID)
	})
	if err != nil {
		return err
	}
	for i := range affected {
		g := &affected[i]
		g.TableID = nil
		g.SeatNum = nil
		s.sseHub.publishGuest("update", g, weddingID)
	}
	return nil
}
