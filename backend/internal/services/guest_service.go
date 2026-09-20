package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/utils"
)

type GuestService struct {
	guestRepo *repository.GuestRepo
	tableRepo *repository.TableRepo
	sseHub    *SSEHub
	db        *gorm.DB
}

func NewGuestService(guestRepo *repository.GuestRepo, tableRepo *repository.TableRepo, sseHub *SSEHub, db *gorm.DB) *GuestService {
	return &GuestService{guestRepo: guestRepo, tableRepo: tableRepo, sseHub: sseHub, db: db}
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
		if errors.Is(err, repository.ErrAlreadyCheckedIn) {
			return ErrAlreadyCheckedIn
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
	// Buffer SSE events and publish them only after the transaction commits,
	// so a rolled-back batch never broadcasts phantom rows.
	type pendingEvent struct {
		eventType string
		guest     *models.GuestRecord
	}
	events := make([]pendingEvent, 0, len(guests))

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGuestRepo := repository.NewGuestRepo(tx)
		txTableRepo := repository.NewTableRepo(tx)
		created = 0
		events = events[:0]
		existing := make(map[uuid.UUID][]models.GuestRecord)
		// Cache table lookups for this batch
		tableCache := make(map[uuid.UUID]*models.BanquetTable)
		for i := range guests {
			g := &guests[i]
			if g.Pax < 1 {
				g.Pax = 1
			}
			if g.TableID == nil || g.SeatNum == nil {
				if err := txGuestRepo.Create(ctx, g); err != nil {
					return err
				}
				events = append(events, pendingEvent{"create", g})
				created++
				continue
			}
			tid := *g.TableID
			// Validate table exists and belongs to this wedding
			if _, ok := tableCache[tid]; !ok {
				t, err := txTableRepo.FindByID(ctx, tid, g.WeddingID)
				if err != nil {
					return fmt.Errorf("table %s not found", tid.String())
				}
				tableCache[tid] = t
			}
			table := tableCache[tid]
			guestEnd := *g.SeatNum + g.Pax - 1
			if *g.SeatNum < 1 || guestEnd > table.Capacity {
				return fmt.Errorf("seat %d-%d on table %s exceeds capacity %d", *g.SeatNum, guestEnd, table.Name, table.Capacity)
			}
			if _, ok := existing[tid]; !ok {
				rows, err := txGuestRepo.FindByTable(ctx, g.WeddingID, tid)
				if err != nil {
					return err
				}
				existing[tid] = rows
			}
			for _, e := range existing[tid] {
				if e.SeatNum == nil {
					continue
				}
				eEnd := *e.SeatNum + e.Pax - 1
				if *g.SeatNum <= eEnd && *e.SeatNum <= guestEnd {
					return fmt.Errorf("seat %d-%d on table overlaps with \"%s\" (seats %d-%d)", *g.SeatNum, guestEnd, e.Name, *e.SeatNum, eEnd)
				}
			}
			existing[tid] = append(existing[tid], *g)
			if err := txGuestRepo.Create(ctx, g); err != nil {
				return err
			}
			events = append(events, pendingEvent{"create", g})
			created++
		}
		return nil
	})
	if err != nil {
		// Transaction rolled back: nothing committed, nothing to broadcast.
		return 0, err
	}
	for _, e := range events {
		s.publishEvent(e.eventType, e.guest, e.guest.WeddingID)
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

// Sync types for offline queue

type SyncOp string

const (
	SyncOpCreate   SyncOp = "create"
	SyncOpUpdate   SyncOp = "update"
	SyncOpDelete   SyncOp = "delete"
	SyncOpCheckIn  SyncOp = "checkin"
	SyncOpCheckOut SyncOp = "checkout"
)

type SyncPayload struct {
	Name      string   `json:"name"`
	Phone     string   `json:"phone"`
	Email     string   `json:"email"`
	Pax       int      `json:"pax"`
	RSVP      string   `json:"rsvp"`
	IsVip     bool     `json:"isVip"`
	Notes     string   `json:"notes"`
	Dietary   []string `json:"dietary"`
	TableID   *string  `json:"tableId"`
	SeatNum   *int     `json:"seatNum"`
	AngbaoAmt *int     `json:"angbaoAmt"`
	GiftItem  *string  `json:"giftItem"`
}

type SyncMutation struct {
	MutationID      string       `json:"mutationId"`
	Op              SyncOp       `json:"op"`
	GuestID         string       `json:"guestId"`
	ClientUpdatedAt time.Time    `json:"clientUpdatedAt"`
	BaseUpdatedAt   *time.Time   `json:"baseUpdatedAt"`
	Payload         *SyncPayload `json:"payload"`
}

type SyncResult struct {
	GuestID      string              `json:"guestId"`
	Status       string              `json:"status"` // applied | skipped | failed
	Reason       string              `json:"reason,omitempty"`
	ServerRecord *models.GuestRecord `json:"serverRecord,omitempty"`
}

// Sync applies a batch of mutations with newer-time-wins.
// Mutations are processed in order received, FIFO.
func (s *GuestService) Sync(ctx context.Context, weddingID uuid.UUID, mutations []SyncMutation) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(mutations))
	for _, m := range mutations {
		res := s.applySyncMutation(ctx, weddingID, m)
		results = append(results, res)
	}
	return results, nil
}

func (s *GuestService) applySyncMutation(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	gid, err := parseSyncID(m.GuestID)
	if err != nil {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "invalid guestId"}
	}

	// Skew clamp: never trust a client clock in the future, or a skewed device
	// would win every LWW comparison from then on.
	opTime := m.ClientUpdatedAt
	if opTime.After(time.Now()) {
		opTime = time.Now()
	}

	// Load server record if exists
	existing, err := s.guestRepo.FindByID(ctx, gid, weddingID)
	found := err == nil

	switch m.Op {
	case SyncOpCreate:
		if found {
			// treat as update with LWW; equal timestamps count as newer (tie-break)
			if opTime.Before(existing.UpdatedAt) {
				return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
			}
			if m.Payload != nil {
				if err := s.validateSyncTableSeat(ctx, weddingID, m.Payload); err != nil {
					return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
				}
				applySyncPayload(existing, m.Payload, &gid)
			}
			existing.UpdatedAt = opTime
			if err := s.guestRepo.SyncUpdate(ctx, existing); err != nil {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
			}
			s.publishEvent("update", existing, weddingID)
			return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}
		}
		if m.Payload == nil || m.Payload.Name == "" {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "name required"}
		}
		if m.Payload.Pax < 1 {
			m.Payload.Pax = 1
		}
		if err := s.validateSyncTableSeat(ctx, weddingID, m.Payload); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		g := &models.GuestRecord{
			ID:        gid,
			WeddingID: weddingID,
			Name:      m.Payload.Name,
			Phone:     m.Payload.Phone,
			Email:     m.Payload.Email,
			Pax:       m.Payload.Pax,
			RSVP:      m.Payload.RSVP,
			IsVip:     m.Payload.IsVip,
			Notes:     m.Payload.Notes,
			Dietary:   m.Payload.Dietary,
			AngbaoAmt: m.Payload.AngbaoAmt,
			GiftItem:  m.Payload.GiftItem,
			CreatedAt: opTime,
			UpdatedAt: opTime,
		}
		if m.Payload.TableID != nil && *m.Payload.TableID != "" {
			if tid, err := parseSyncID(*m.Payload.TableID); err == nil {
				g.TableID = &tid
			}
		}
		g.SeatNum = m.Payload.SeatNum
		if err := s.guestRepo.SyncCreate(ctx, g); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		s.publishEvent("create", g, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}

	case SyncOpUpdate:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found", ServerRecord: nil}
		}
		// Equal timestamps win the tie-break: only strictly older ops lose.
		if opTime.Before(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
		}
		if m.Payload != nil {
			if err := s.validateSyncTableSeat(ctx, weddingID, m.Payload); err != nil {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
			}
			applySyncPayload(existing, m.Payload, nil)
		}
		existing.UpdatedAt = opTime
		if err := s.guestRepo.SyncUpdate(ctx, existing); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		s.publishEvent("update", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}

	case SyncOpDelete:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "applied"}
		}
		if opTime.Before(existing.UpdatedAt) {
			// Report "failed", not "skipped": the client keeps failed items
			// queued instead of dequeuing and losing the delete intent.
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "older than server", ServerRecord: existing}
		}
		if err := s.guestRepo.Delete(ctx, gid, weddingID); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		s.publishEvent("delete", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied"}

	case SyncOpCheckIn:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if opTime.Before(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
		}
		// Same walk-in promotion as GuestService.CheckIn: RSVP follows attendance.
		if existing.RSVP != "confirmed" {
			existing.WalkIn = true
			existing.RSVP = "confirmed"
		}
		existing.CheckedInAt = &opTime
		if m.Payload != nil {
			if m.Payload.AngbaoAmt != nil {
				existing.AngbaoAmt = m.Payload.AngbaoAmt
			}
			if m.Payload.GiftItem != nil {
				existing.GiftItem = m.Payload.GiftItem
			}
		}
		existing.UpdatedAt = opTime
		if err := s.guestRepo.SyncUpdate(ctx, existing); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		s.publishEvent("checkin", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}

	case SyncOpCheckOut:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if opTime.Before(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
		}
		existing.CheckedInAt = nil
		existing.UpdatedAt = opTime
		if err := s.guestRepo.SyncUpdate(ctx, existing); err != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
		}
		s.publishEvent("checkout", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}

	default:
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "unknown op"}
	}
}

// validateSyncTableSeat mirrors the table/seat rules of AssignSeat for offline
// sync mutations: the table must exist in this wedding and the seat must fit
// its capacity. A nil/empty tableId (unassign) needs no validation.
func (s *GuestService) validateSyncTableSeat(ctx context.Context, weddingID uuid.UUID, p *SyncPayload) error {
	if p.TableID == nil || *p.TableID == "" {
		return nil
	}
	tid, err := parseSyncID(*p.TableID)
	if err != nil {
		return fmt.Errorf("invalid tableId %q", *p.TableID)
	}
	table, err := s.tableRepo.FindByID(ctx, tid, weddingID)
	if err != nil {
		return fmt.Errorf("table %s not found", *p.TableID)
	}
	if p.SeatNum != nil && table.Capacity > 0 && *p.SeatNum > table.Capacity {
		return fmt.Errorf("seat %d exceeds capacity %d of table %s", *p.SeatNum, table.Capacity, table.Name)
	}
	return nil
}

func applySyncPayload(g *models.GuestRecord, p *SyncPayload, gid *uuid.UUID) {
	if p.Name != "" {
		g.Name = p.Name
	}
	g.Phone = p.Phone
	g.Email = p.Email
	if p.Pax >= 1 {
		g.Pax = p.Pax
	}
	if p.RSVP != "" {
		g.RSVP = p.RSVP
	}
	g.IsVip = p.IsVip
	g.Notes = p.Notes
	g.Dietary = p.Dietary
	g.AngbaoAmt = p.AngbaoAmt
	g.GiftItem = p.GiftItem
	switch {
	case p.TableID == nil || *p.TableID == "":
		// JSON null and empty string both mean "unassign": clear the whole
		// seat assignment. SeatNum must never survive with a nil TableID.
		g.TableID = nil
		g.SeatNum = nil
	default:
		if tid, err := parseSyncID(*p.TableID); err == nil {
			g.TableID = &tid
			g.SeatNum = p.SeatNum
		}
	}
	if gid != nil {
		g.ID = *gid
	}
}

func parseSyncID(s string) (uuid.UUID, error) {
	if id, err := uuid.Parse(s); err == nil {
		return id, nil
	}
	// try base64 url
	return parseBase64ID(s)
}

func parseBase64ID(s string) (uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			return uuid.Nil, err
		}
	}
	return uuid.FromBytes(decoded)
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
		WalkIn:  g.WalkIn,
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
