package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/utils"
)

type GuestService struct {
	guestRepo *repository.GuestRepo
	tableRepo *repository.TableRepo
	sseHub    *SSEHub
}

func NewGuestService(guestRepo *repository.GuestRepo, tableRepo *repository.TableRepo, sseHub *SSEHub) *GuestService {
	return &GuestService{guestRepo: guestRepo, tableRepo: tableRepo, sseHub: sseHub}
}

func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(ctx, weddingID, cursor, limit)
}

func (s *GuestService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(ctx, id, weddingID)
}

func (s *GuestService) Create(ctx context.Context, g *models.GuestRecord) error {
	if err := s.guestRepo.Create(ctx, g); err != nil {
		return err
	}
	s.publishEvent("create", g, g.WeddingID)
	return nil
}

func (s *GuestService) Update(ctx context.Context, g *models.GuestRecord) error {
	if err := s.guestRepo.Update(ctx, g); err != nil {
		return err
	}
	s.publishEvent("update", g, g.WeddingID)
	return nil
}

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Fetch guest before deletion so we can broadcast the event.
	guest, _ := s.guestRepo.FindByID(ctx, id, weddingID)
	if err := s.guestRepo.Delete(ctx, id, weddingID); err != nil {
		return err
	}
	if guest != nil {
		s.publishEvent("delete", guest, weddingID)
	}
	return nil
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
	if guest.Pax < 1 {
		return errors.New("guest pax must be at least 1")
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
	if err := s.guestRepo.Update(ctx, guest); err != nil {
		return err
	}
	s.publishEvent("seat_assign", guest, weddingID)
	return nil
}

// CheckIn marks a guest as checked in. Returns ErrAlreadyCheckedIn if the guest
// was already checked in by another receptionist (FIFO conflict).
func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID) error {
	now := time.Now()
	// Atomic conditional update: only check in if not already checked in
	if err := s.guestRepo.ConditionalCheckIn(ctx, id, weddingID, now); err != nil {
		if errors.Is(err, models.ErrAlreadyCheckedIn) {
			return models.ErrAlreadyCheckedIn
		}
		return err
	}
	guest, _ := s.guestRepo.FindByID(ctx, id, weddingID)
	if guest != nil {
		s.publishEvent("checkin", guest, weddingID)
	}
	return nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		return err
	}
	guest.CheckedInAt = nil
	if err := s.guestRepo.Update(ctx, guest); err != nil {
		return err
	}
	s.publishEvent("checkout", guest, weddingID)
	return nil
}

func (s *GuestService) BulkCreate(ctx context.Context, guests []models.GuestRecord) (int, error) {
	created := 0
	existing := make(map[uuid.UUID][]models.GuestRecord)
	// Cache table lookups for this batch
	tableCache := make(map[uuid.UUID]*models.BanquetTable)
	for i := range guests {
		g := &guests[i]
		if g.Pax < 1 {
			g.Pax = 1
		}
		if g.TableID == nil || g.SeatNum == nil {
			if err := s.guestRepo.Create(ctx, g); err != nil {
				return created, err
			}
			s.publishEvent("create", g, g.WeddingID)
			created++
			continue
		}
		tid := *g.TableID
		// Validate table exists and belongs to this wedding
		if _, ok := tableCache[tid]; !ok {
			t, err := s.tableRepo.FindByID(ctx, tid, g.WeddingID)
			if err != nil {
				return created, fmt.Errorf("table %s not found", tid.String())
			}
			tableCache[tid] = t
		}
		table := tableCache[tid]
		guestEnd := *g.SeatNum + g.Pax - 1
		if *g.SeatNum < 1 || guestEnd > table.Capacity {
			return created, fmt.Errorf("seat %d-%d on table %s exceeds capacity %d", *g.SeatNum, guestEnd, table.Name, table.Capacity)
		}
		if _, ok := existing[tid]; !ok {
			rows, err := s.guestRepo.FindByTable(ctx, g.WeddingID, tid)
			if err != nil {
				return created, err
			}
			existing[tid] = rows
		}
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
		s.publishEvent("create", g, g.WeddingID)
		created++
	}
	return created, nil
}

func (s *GuestService) Occupancy(ctx context.Context, weddingID uuid.UUID) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(ctx, weddingID)
}

// publishEvent broadcasts a guest mutation to all connected SSE clients via Redis.
// Uses context.Background() because the service layer does not yet propagate request
// context. The goroutine ensures publishing never blocks the caller.
func (s *GuestService) publishEvent(eventType string, guest *models.GuestRecord, weddingID uuid.UUID) {
	if s.sseHub == nil {
		return
	}

	event := GuestEvent{
		Type:      eventType,
		GuestID:   guest.ID.String(),
		WeddingID: weddingID.String(),
		Guest:     guestToEventData(guest),
		Timestamp: time.Now().UnixMilli(),
	}

	go func() {
		if err := s.sseHub.Publish(context.Background(), weddingID, event); err != nil {
			log.Printf("SSE: publish %s for guest %s in wedding %s: %v", eventType, guest.ID, weddingID, err)
		}
	}()
}

func guestToEventData(g *models.GuestRecord) *GuestEventData {
	d := &GuestEventData{
		ID:      utils.EncodeUUID(g.ID),
		Name:    g.Name,
		Phone:   g.Phone,
		Email:   g.Email,
		Pax:     g.Pax,
		RSVP:    g.RSVP,
		IsVip:   g.IsVip,
		Notes:   g.Notes,
		Dietary: []string(g.Dietary),
	}
	if g.TableID != nil {
		id := utils.EncodeUUID(*g.TableID)
		d.TableID = &id
	}
	if g.SeatNum != nil {
		d.SeatNum = g.SeatNum
	}
	if g.CheckedInAt != nil {
		t := g.CheckedInAt.Format(time.RFC3339)
		d.CheckedInAt = &t
	}
	if g.AngbaoAmt != nil {
		d.AngbaoAmt = g.AngbaoAmt
	}
	if g.GiftItem != nil {
		d.GiftItem = g.GiftItem
	}
	return d
}
