package services

import (
	"context"
	"errors"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TableService struct {
	tableRepo *repository.TableRepo
	sseHub    *SSEHub
	db        *gorm.DB
}

func NewTableService(tableRepo *repository.TableRepo, sseHub *SSEHub, db *gorm.DB) *TableService {
	return &TableService{tableRepo: tableRepo, sseHub: sseHub, db: db}
}

func (s *TableService) List(ctx context.Context, weddingID uuid.UUID) ([]models.BanquetTable, error) {
	return s.tableRepo.ListByWedding(ctx, weddingID)
}

func (s *TableService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.BanquetTable, error) {
	return s.tableRepo.FindByID(ctx, id, weddingID)
}

func (s *TableService) Create(ctx context.Context, t *models.BanquetTable) error {
	// There is no foreign key on wedding_id, so verify the wedding exists (under
	// the same lock guest writes take) before inserting an orphan table.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWedding(tx, t.WeddingID); err != nil {
			return err
		}
		var wedding models.WeddingEvent
		if err := tx.Select("id").Where("id = ?", t.WeddingID).First(&wedding).Error; err != nil {
			return err
		}
		return repository.NewTableRepo(tx).Create(ctx, t)
	})
}

func (s *TableService) Update(ctx context.Context, t *models.BanquetTable) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWedding(tx, t.WeddingID); err != nil {
			return err
		}
		if err := lockTable(tx, t.WeddingID, t.ID); err != nil {
			return err
		}
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
		// Wedding lock, then table lock: the order every guest write uses, so a
		// concurrent seat move either lands before the unassign or sees no table.
		if err := lockWedding(tx, weddingID); err != nil {
			return err
		}
		if err := lockTable(tx, weddingID, id); err != nil {
			return err
		}
		guests := repository.NewGuestRepo(tx)
		var err error
		// Lock the affected rows before snapshotting them: a check-in that
		// commits while this runs must either land before the read or wait for
		// it, never be reverted by the per-row write below.
		if affected, err = guests.FindByTableForUpdate(ctx, weddingID, id); err != nil {
			return err
		}
		// Unseating carries the tableId group, so it goes through the shared
		// merge: the group's field_times records this operation and an older
		// queued move loses instead of re-seating the guest on a deleted table.
		now := time.Now()
		for i := range affected {
			g := &affected[i]
			candidate := *g
			candidate.TableID = nil
			candidate.SeatNum = nil
			if won := mergeGuestWrite(g, &candidate, now, []string{groupTableID}); len(won) == 0 {
				continue
			}
			if err := guests.Update(ctx, g); err != nil {
				return err
			}
		}
		return repository.NewTableRepo(tx).Delete(ctx, id, weddingID)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTableNotFound
		}
		return err
	}
	for i := range affected {
		s.sseHub.publishGuest("update", &affected[i], weddingID)
	}
	return nil
}
