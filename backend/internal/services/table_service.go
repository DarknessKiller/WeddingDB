package services

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type TableService struct {
	tableRepo *repository.TableRepo
	guestRepo *repository.GuestRepo
	sseHub    *SSEHub
}

func NewTableService(tableRepo *repository.TableRepo, guestRepo *repository.GuestRepo, sseHub *SSEHub) *TableService {
	return &TableService{tableRepo: tableRepo, guestRepo: guestRepo, sseHub: sseHub}
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
	// Surface the guests seated at this table before unassigning so their
	// updates can be broadcast afterwards.
	affected, err := s.guestRepo.FindByTable(ctx, weddingID, id)
	if err != nil {
		return err
	}
	// Unassign guests from this table before deleting
	if err := s.guestRepo.UnassignByTable(ctx, weddingID, id); err != nil {
		return err
	}
	if err := s.tableRepo.Delete(ctx, id, weddingID); err != nil {
		return err
	}
	for i := range affected {
		g := &affected[i]
		g.TableID = nil
		g.SeatNum = nil
		s.publishGuestEvent("update", g, weddingID)
	}
	return nil
}

// publishGuestEvent broadcasts a guest mutation to all connected SSE clients,
// mirroring GuestService.publishEvent (same hub, envelope and payload shape).
func (s *TableService) publishGuestEvent(eventType string, guest *models.GuestRecord, weddingID uuid.UUID) {
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
