package services

import (
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

func (s *GuestService) List(weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(weddingID, cursor, limit)
}

func (s *GuestService) Get(id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(id, weddingID)
}

func (s *GuestService) Create(g *models.GuestRecord) error {
	return s.guestRepo.Create(g)
}

func (s *GuestService) Update(g *models.GuestRecord) error {
	return s.guestRepo.Update(g)
}

func (s *GuestService) Delete(id, weddingID uuid.UUID) error {
	return s.guestRepo.Delete(id, weddingID)
}

func (s *GuestService) Search(weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	return s.guestRepo.SearchByWedding(weddingID, query)
}

func (s *GuestService) AssignSeat(guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	guest, err := s.guestRepo.FindByID(guestID, weddingID)
	if err != nil {
		return err
	}
	table, err := s.tableRepo.FindByID(tableID, weddingID)
	if err != nil {
		return errors.New("table not found")
	}
	if seatNum < 1 || seatNum+guest.Pax-1 > table.Capacity {
		return errors.New("seat range exceeds table capacity")
	}
	existing, err := s.guestRepo.FindByTable(weddingID, tableID)
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
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckIn(id, weddingID uuid.UUID) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	now := time.Now()
	guest.CheckedInAt = &now
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckOut(id, weddingID uuid.UUID) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	guest.CheckedInAt = nil
	return s.guestRepo.Update(guest)
}

func (s *GuestService) BulkCreate(guests []models.GuestRecord) (int, error) {
	created := 0
	existing := make(map[uuid.UUID][]models.GuestRecord)
	for i := range guests {
		g := &guests[i]
		if g.TableID == nil || g.SeatNum == nil {
			if err := s.guestRepo.Create(g); err != nil {
				return created, err
			}
			created++
			continue
		}
		tid := *g.TableID
		if _, ok := existing[tid]; !ok {
			rows, err := s.guestRepo.FindByTable(g.WeddingID, tid)
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
		if err := s.guestRepo.Create(g); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func (s *GuestService) Occupancy(weddingID uuid.UUID) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(weddingID)
}
