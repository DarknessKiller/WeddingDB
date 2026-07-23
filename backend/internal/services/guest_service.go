package services

import (
	"errors"
	"time"
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

func (s *GuestService) List(weddingID uint, offset, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(weddingID, offset, limit)
}

func (s *GuestService) Get(id, weddingID uint) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(id, weddingID)
}

func (s *GuestService) Create(g *models.GuestRecord) error {
	return s.guestRepo.Create(g)
}

func (s *GuestService) Update(g *models.GuestRecord) error {
	return s.guestRepo.Update(g)
}

func (s *GuestService) Delete(id, weddingID uint) error {
	return s.guestRepo.Delete(id, weddingID)
}

func (s *GuestService) Search(weddingID uint, query string) ([]models.GuestRecord, error) {
	return s.guestRepo.SearchByWedding(weddingID, query)
}

func (s *GuestService) AssignSeat(guestID, weddingID, tableID uint, seatNum int) error {
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
	guest.TableID = &tableID
	guest.SeatNum = &seatNum
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckIn(id, weddingID uint) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	now := time.Now()
	guest.CheckedInAt = &now
	return s.guestRepo.Update(guest)
}

func (s *GuestService) CheckOut(id, weddingID uint) error {
	guest, err := s.guestRepo.FindByID(id, weddingID)
	if err != nil {
		return err
	}
	guest.CheckedInAt = nil
	return s.guestRepo.Update(guest)
}

func (s *GuestService) Occupancy(weddingID uint) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(weddingID)
}
