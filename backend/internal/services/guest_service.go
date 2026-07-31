package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type GuestService struct {
	guestRepo *repository.GuestRepo
	tableRepo *repository.TableRepo
}

func NewGuestService(guestRepo *repository.GuestRepo, tableRepo *repository.TableRepo) *GuestService {
	return &GuestService{guestRepo: guestRepo, tableRepo: tableRepo}
}

func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(ctx, weddingID, cursor, limit)
}

func (s *GuestService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(ctx, id, weddingID)
}

func (s *GuestService) Create(ctx context.Context, g *models.GuestRecord) error {
	return s.guestRepo.Create(ctx, g)
}

func (s *GuestService) Update(ctx context.Context, g *models.GuestRecord) error {
	return s.guestRepo.Update(ctx, g)
}

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return s.guestRepo.Delete(ctx, id, weddingID)
}

func (s *GuestService) Search(ctx context.Context, weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	return s.guestRepo.SearchByWedding(ctx, weddingID, query)
}

func (s *GuestService) AssignSeat(ctx context.Context, guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	guest, err := s.guestRepo.FindByID(ctx, guestID, weddingID)
	if err != nil {
		return err
	}
	table, err := s.tableRepo.FindByID(ctx, tableID, weddingID)
	if err != nil {
		return errors.New("table not found")
	}
	if seatNum < 1 || seatNum+guest.Pax-1 > table.Capacity {
		return errors.New("seat range exceeds table capacity")
	}
	existing, err := s.guestRepo.FindByTable(ctx, weddingID, tableID)
	if err != nil {
		return err
	}
	guestEnd := seatNum + guest.Pax - 1
	for _, e := range existing {
		if e.ID == guestID {
			continue
		}
		if e.SeatNum == nil {
			continue
		}
		eEnd := *e.SeatNum + e.Pax - 1
		if seatNum <= eEnd && *e.SeatNum <= guestEnd {
			return errors.New("seat overlaps with another guest")
		}
	}
	guest.TableID = &tableID
	guest.SeatNum = &seatNum
	return s.guestRepo.Update(ctx, guest)
}

func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID) error {
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		return err
	}
	now := time.Now()
	guest.CheckedInAt = &now
	return s.guestRepo.Update(ctx, guest)
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		return err
	}
	guest.CheckedInAt = nil
	return s.guestRepo.Update(ctx, guest)
}

func (s *GuestService) BulkCreate(ctx context.Context, guests []models.GuestRecord) (int, error) {
	created := 0
	existing := make(map[uuid.UUID][]models.GuestRecord)
	for i := range guests {
		g := &guests[i]
		if g.TableID == nil || g.SeatNum == nil {
			if err := s.guestRepo.Create(ctx, g); err != nil {
				return created, err
			}
			created++
			continue
		}
		tid := *g.TableID
		if _, ok := existing[tid]; !ok {
			rows, err := s.guestRepo.FindByTable(ctx, g.WeddingID, tid)
			if err != nil {
				return created, err
			}
			existing[tid] = rows
		}
		guestEnd := *g.SeatNum + g.Pax - 1
		for _, e := range existing[tid] {
			if e.SeatNum == nil {
				continue
			}
			eEnd := *e.SeatNum + e.Pax - 1
			if *g.SeatNum <= eEnd && *e.SeatNum <= guestEnd {
				return created, fmt.Errorf("seat %d-%d on table overlaps with \"%s\" (seats %d-%d)", *g.SeatNum, guestEnd, e.Name, *e.SeatNum, eEnd)
			}
		}
		existing[tid] = append(existing[tid], *g)
		if err := s.guestRepo.Create(ctx, g); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func (s *GuestService) Occupancy(ctx context.Context, weddingID uuid.UUID) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(ctx, weddingID)
}
