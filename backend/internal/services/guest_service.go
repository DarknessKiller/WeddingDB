package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// MaxGuestsPerWedding caps a wedding at 1000 guests. Party size counts: a
// guest entry with pax 2 counts as two guests.
const MaxGuestsPerWedding = 1000

// MaxGuestPartySize bounds one guest entry's pax. Without a bound, huge values
// overflow the cap arithmetic and break every later guest count and occupancy
// query for the wedding.
const MaxGuestPartySize = 50

// ErrGuestCapExceeded is returned when adding guests would cross the cap.
var ErrGuestCapExceeded = errors.New("guest limit exceeded")

func guestCapError(current, delta int) error {
	return fmt.Errorf("%w: this wedding has %d guests; this change would make %d, over the %d-guest limit",
		ErrGuestCapExceeded, current, current+delta, MaxGuestsPerWedding)
}

// guestCountLocked serializes guest additions for one wedding and returns the
// current count. The row lock is Postgres-only; sqlite (tests) has one writer.
func (s *GuestService) guestCountLocked(tx *gorm.DB, weddingID uuid.UUID) (int, error) {
	if tx.Name() == "postgres" {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").Where("id = ?", weddingID).First(&models.WeddingEvent{}).Error; err != nil {
			return 0, err
		}
	}
	var total int
	if err := tx.Model(&models.GuestRecord{}).
		Where("wedding_id = ?", weddingID).
		Select("COALESCE(SUM(pax), 0)").Row().Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *GuestService) createGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord, syncWrite bool) error {
	if g.Pax < 1 {
		g.Pax = 1
	}
	if g.Pax > MaxGuestPartySize {
		return fmt.Errorf("party size must be at most %d", MaxGuestPartySize)
	}
	if err := ValidateGuestText(g); err != nil {
		return err
	}
	// Wedding row first, table row second: every write path locks in this
	// order, so the two lock sets cannot deadlock.
	total, err := s.guestCountLocked(tx, g.WeddingID)
	if err != nil {
		return err
	}
	if err := checkSeatNotTaken(ctx, tx, g); err != nil {
		return err
	}
	if total+g.Pax > MaxGuestsPerWedding {
		return guestCapError(total, g.Pax)
	}
	repo := repository.NewGuestRepo(tx)
	if syncWrite {
		return repo.SyncCreate(ctx, g)
	}
	return repo.Create(ctx, g)
}

func (s *GuestService) updateGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord, syncWrite bool) error {
	if g.Pax < 1 {
		g.Pax = 1
	}
	if g.Pax > MaxGuestPartySize {
		return fmt.Errorf("party size must be at most %d", MaxGuestPartySize)
	}
	if err := ValidateGuestText(g); err != nil {
		return err
	}
	total, err := s.guestCountLocked(tx, g.WeddingID)
	if err != nil {
		return err
	}
	if err := checkSeatNotTaken(ctx, tx, g); err != nil {
		return err
	}
	var current models.GuestRecord
	if err := tx.Select("id", "pax", "checked_in_at").
		Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID).
		First(&current).Error; err != nil {
		return err
	}
	if g.Pax > current.Pax && total-current.Pax+g.Pax > MaxGuestsPerWedding {
		return guestCapError(total, g.Pax-current.Pax)
	}
	repo := repository.NewGuestRepo(tx)
	if syncWrite {
		return repo.SyncUpdate(ctx, g)
	}
	// Online edits never write checked_in_at; report the stored value.
	g.CheckedInAt = current.CheckedInAt
	return repo.Update(ctx, g)
}

func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(ctx, weddingID, cursor, limit)
}

func (s *GuestService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(ctx, id, weddingID)
}

// ErrGuestIDTaken is returned when a client-supplied guest ID belongs to a
// guest of another wedding.
var ErrGuestIDTaken = errors.New("guest ID already in use")

// CreateOrGet creates g, or returns the stored guest when g.ID already exists
// in g's wedding, so a retried create (lost response, offline replay) is a
// no-op. created reports which happened.
func (s *GuestService) CreateOrGet(ctx context.Context, g *models.GuestRecord) (guest *models.GuestRecord, created bool, err error) {
	if g.ID != uuid.Nil {
		if existing, err := s.guestRepo.FindByID(ctx, g.ID, g.WeddingID); err == nil {
			return existing, false, nil
		}
	}
	createErr := s.Create(ctx, g)
	if createErr == nil || g.ID == uuid.Nil {
		return g, createErr == nil, createErr
	}
	// A concurrent create with the same ID may have won the insert.
	if existing, err := s.guestRepo.FindByID(ctx, g.ID, g.WeddingID); err == nil {
		return existing, false, nil
	}
	inUse, err := s.guestRepo.IDInUse(ctx, g.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check guest id: %w", err)
	}
	if inUse {
		return nil, false, ErrGuestIDTaken
	}
	return nil, false, createErr
}

func (s *GuestService) Create(ctx context.Context, g *models.GuestRecord) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.createGuestTx(ctx, tx, g, false)
	})
	if err != nil {
		return err
	}
	s.publishEvent("create", g, g.WeddingID)
	return nil
}

func (s *GuestService) Update(ctx context.Context, g *models.GuestRecord) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.updateGuestTx(ctx, tx, g, false)
	})
	if err != nil {
		return err
	}
	s.publishEvent("update", g, g.WeddingID)
	return nil
}

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Fetch guest before deletion so we can broadcast the event.
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		log.Printf("guest lookup before delete failed: %v", err)
	}
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

// checkSeatTx validates a seat assignment inside tx and returns the table row
// lock. Every write path that sets a seat calls this, so all of them serialize
// on the same table row and share the capacity/overlap rules.
// The row lock is Postgres-only; sqlite (tests) has one writer.
func checkSeatTx(ctx context.Context, tx *gorm.DB, weddingID, tableID uuid.UUID, guestID uuid.UUID, pax, seatNum int) error {
	if tx.Name() == "postgres" {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND wedding_id = ?", tableID, weddingID).First(&models.BanquetTable{}).Error; err != nil {
			return errors.New("table not found")
		}
	}
	table, err := repository.NewTableRepo(tx).FindByID(ctx, tableID, weddingID)
	if err != nil {
		return errors.New("table not found")
	}
	if pax < 1 {
		return errors.New("guest pax must be at least 1")
	}
	if seatNum < 1 || seatNum > table.Capacity-pax+1 {
		return errors.New("seat range exceeds table capacity")
	}
	existing, err := repository.NewGuestRepo(tx).FindByTable(ctx, weddingID, tableID)
	if err != nil {
		return err
	}
	guestEnd := seatNum + pax - 1
	for _, e := range existing {
		if e.ID == guestID || e.SeatNum == nil {
			continue
		}
		eEnd := *e.SeatNum + e.Pax - 1
		if seatNum <= eEnd && *e.SeatNum <= guestEnd {
			return fmt.Errorf("%w: \"%s\" (seats %d-%d)", ErrSeatOverlap, e.Name, *e.SeatNum, eEnd)
		}
	}
	return nil
}

// checkSeatNotTaken enforces the seat rules for a whole guest row: a table
// without a seat is a plain table assignment and needs no lock, while a
// table+seat pair goes through the same locked check as AssignSeat.
func checkSeatNotTaken(ctx context.Context, tx *gorm.DB, g *models.GuestRecord) error {
	if g.TableID == nil {
		return nil
	}
	if g.SeatNum == nil {
		// Table without a seat: only the wedding-scoped ownership check applies,
		// but a cross-wedding table id must still be rejected.
		if _, err := repository.NewTableRepo(tx).FindByID(ctx, *g.TableID, g.WeddingID); err != nil {
			return errors.New("table not found")
		}
		return nil
	}
	return checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, g.ID, g.Pax, *g.SeatNum)
}

func (s *GuestService) AssignSeat(ctx context.Context, guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	var guest *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGuestRepo := repository.NewGuestRepo(tx)
		var err error
		guest, err = txGuestRepo.FindByID(ctx, guestID, weddingID)
		if err != nil {
			return err
		}
		// Wedding row lock keeps the lock order identical to create/update, so
		// a concurrent seat move and import cannot deadlock each other.
		if _, err := s.guestCountLocked(tx, weddingID); err != nil {
			return err
		}
		if err := checkSeatTx(ctx, tx, weddingID, tableID, guestID, guest.Pax, seatNum); err != nil {
			return err
		}
		guest.TableID = &tableID
		guest.SeatNum = &seatNum
		return txGuestRepo.UpdateFields(ctx, guestID, weddingID, map[string]any{"table_id": tableID, "seat_num": seatNum})
	})
	if err != nil {
		return err
	}
	s.publishEvent("seat_assign", guest, weddingID)
	return nil
}

// UpdateWithSeat saves edited fields and, when tableID is set, moves the guest
// to that table in the same transaction — a failed seat check leaves the
// fields untouched and a failed field write leaves the seat where it was.
// A nil tableID preserves the current seat assignment.
func (s *GuestService) UpdateWithSeat(ctx context.Context, g *models.GuestRecord, tableID *uuid.UUID, seatNum int) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tableID != nil {
			g.TableID = tableID
			g.SeatNum = &seatNum
		}
		return s.updateGuestTx(ctx, tx, g, false)
	})
	if err != nil {
		return err
	}
	s.publishEvent("update", g, g.WeddingID)
	return nil
}

// CheckIn marks a guest as checked in and promotes their RSVP to confirmed.
// Non-empty notes are appended to the guest's existing notes.
// Returns ErrAlreadyCheckedIn if the guest was already checked in by another
// receptionist (FIFO conflict).
func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID, notes string) error {
	now := time.Now()
	updatedNotes := ""
	notes = strings.TrimSpace(notes)
	if notes != "" {
		existing, err := s.guestRepo.FindByID(ctx, id, weddingID)
		if err != nil {
			return err
		}
		updatedNotes = appendNote(existing.Notes, notes)
	}
	// Atomic conditional update: only check in if not already checked in
	if err := s.guestRepo.ConditionalCheckIn(ctx, id, weddingID, now, updatedNotes); err != nil {
		if errors.Is(err, repository.ErrAlreadyCheckedIn) {
			return ErrAlreadyCheckedIn
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		log.Printf("guest lookup after check-in failed: %v", err)
	}
	if guest != nil {
		s.publishEvent("checkin", guest, weddingID)
	}
	return nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	if err := s.guestRepo.UpdateFields(ctx, id, weddingID, map[string]any{"checked_in_at": nil}); err != nil {
		return err
	}
	s.publishFresh(ctx, "checkout", id, weddingID)
	return nil
}

// UnassignSeat clears the guest's table and seat without touching other columns.
func (s *GuestService) UnassignSeat(ctx context.Context, id, weddingID uuid.UUID) error {
	if err := s.guestRepo.UpdateFields(ctx, id, weddingID, map[string]any{"table_id": nil, "seat_num": nil}); err != nil {
		return err
	}
	s.publishFresh(ctx, "update", id, weddingID)
	return nil
}

// publishFresh re-reads the guest after a column-targeted write and broadcasts
// it. The write already committed, so a failed read is only logged.
func (s *GuestService) publishFresh(ctx context.Context, eventType string, id, weddingID uuid.UUID) {
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		log.Printf("guest lookup after %s failed: %v", eventType, err)
		return
	}
	s.publishEvent(eventType, guest, weddingID)
}

func (s *GuestService) BulkCreate(ctx context.Context, guests []models.GuestRecord) (int, error) {
	created := 0
	if len(guests) == 0 {
		return 0, nil
	}
	weddingID := guests[0].WeddingID
	incoming := 0
	for i := range guests {
		if guests[i].Pax < 1 {
			guests[i].Pax = 1
		}
		if guests[i].Pax > MaxGuestPartySize {
			return 0, fmt.Errorf("party size must be at most %d", MaxGuestPartySize)
		}
		if err := ValidateGuestText(&guests[i]); err != nil {
			return 0, err
		}
		incoming += guests[i].Pax
	}
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
		total, err := s.guestCountLocked(tx, weddingID)
		if err != nil {
			return err
		}
		if total+incoming > MaxGuestsPerWedding {
			return guestCapError(total, incoming)
		}
		// Cache table ownership checks for this batch. Seat rows are row-locked
		// by checkSeatTx, so an import serializes on the same rows as AssignSeat.
		checked := make(map[uuid.UUID]bool)
		for i := range guests {
			g := &guests[i]
			if g.Pax < 1 {
				g.Pax = 1
			}
			if g.TableID == nil {
				if err := txGuestRepo.Create(ctx, g); err != nil {
					return err
				}
				events = append(events, pendingEvent{"create", g})
				created++
				continue
			}
			tid := *g.TableID
			// A table without a seat is a plain table assignment; still verify
			// ownership so an import cannot attach another wedding's table.
			if g.SeatNum == nil {
				if !checked[tid] {
					if _, err := txTableRepo.FindByID(ctx, tid, g.WeddingID); err != nil {
						return fmt.Errorf("table %s not found", tid.String())
					}
					checked[tid] = true
				}
				if err := txGuestRepo.Create(ctx, g); err != nil {
					return err
				}
				events = append(events, pendingEvent{"create", g})
				created++
				continue
			}
			if err := checkSeatTx(ctx, tx, g.WeddingID, tid, uuid.Nil, g.Pax, *g.SeatNum); err != nil {
				return err
			}
			checked[tid] = true
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

	// present holds the JSON keys the client actually sent. A nil map means
	// the payload was built in code, where every field counts as carried.
	// Offline clients send partial payloads, so an absent key must leave the
	// stored value untouched (offline-sync invariant 1).
	present map[string]struct{}
}

func (p *SyncPayload) UnmarshalJSON(data []byte) error {
	type plain SyncPayload
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	*p = SyncPayload(v)
	p.present = make(map[string]struct{}, len(keys))
	for k := range keys {
		p.present[k] = struct{}{}
	}
	return nil
}

func (p *SyncPayload) has(key string) bool {
	if p.present == nil {
		return true
	}
	_, ok := p.present[key]
	return ok
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
		results = append(results, s.applySyncOnce(ctx, weddingID, m))
	}
	return results, nil
}

// maxMutationIDLen bounds the client-sent mutationId (a UUID in practice).
const maxMutationIDLen = 64

// errSyncNotApplied rolls back the marker of a skipped or failed mutation, so
// its retry is evaluated again.
var errSyncNotApplied = errors.New("sync mutation not applied")

// applySyncOnce applies m at most once per mutationId (offline-sync invariant
// 4): the marker row and the guest write commit in one transaction, and a
// replay reports applied without writing. An empty mutationId (older clients)
// skips the dedupe.
// shortcut: markers are kept until the wedding is deleted; prune by age if the table grows large.
func (s *GuestService) applySyncOnce(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	if m.MutationID == "" {
		return s.applySyncMutation(ctx, weddingID, m)
	}
	if len(m.MutationID) > maxMutationIDLen {
		return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "invalid mutationId"}
	}
	var res SyncResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mark := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.AppliedMutation{WeddingID: weddingID, MutationID: m.MutationID})
		if mark.Error != nil {
			return fmt.Errorf("record mutation: %w", mark.Error)
		}
		if mark.RowsAffected == 0 {
			res = SyncResult{GuestID: m.GuestID, Status: "applied"}
			if gid, err := parseSyncID(m.GuestID); err == nil {
				if g, err := repository.NewGuestRepo(tx).FindByID(ctx, gid, weddingID); err == nil {
					res.ServerRecord = g
				}
			}
			return nil
		}
		txSvc := &GuestService{guestRepo: repository.NewGuestRepo(tx), tableRepo: repository.NewTableRepo(tx), sseHub: s.sseHub, db: tx}
		res = txSvc.applySyncMutation(ctx, weddingID, m)
		if res.Status != "applied" {
			return errSyncNotApplied
		}
		return nil
	})
	if err != nil && !errors.Is(err, errSyncNotApplied) {
		return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
	}
	return res
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
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return s.updateGuestTx(ctx, tx, existing, true)
			}); err != nil {
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
		if g.TableID == nil {
			g.SeatNum = nil // a seat without a table is not a valid assignment
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.createGuestTx(ctx, tx, g, true)
		}); err != nil {
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
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.updateGuestTx(ctx, tx, existing, true)
		}); err != nil {
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
		existing.CheckedInAt = &opTime
		existing.RSVP = "confirmed"
		if m.Payload != nil {
			if m.Payload.AngbaoAmt != nil {
				existing.AngbaoAmt = m.Payload.AngbaoAmt
			}
			if m.Payload.GiftItem != nil {
				existing.GiftItem = m.Payload.GiftItem
			}
			// A notes value in a checkin payload is appended to the guest's notes,
			// never replacing them, so a queued note never wipes existing notes.
			note := strings.TrimSpace(m.Payload.Notes)
			if err := checkLen("Check-in notes", note, MaxCheckInNoteLen); err != nil {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
			}
			existing.Notes = appendNote(existing.Notes, note)
			if err := ValidateGuestText(existing); err != nil {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
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
	if p.SeatNum != nil && table.Capacity > 0 && (*p.SeatNum < 1 || *p.SeatNum > table.Capacity) {
		return fmt.Errorf("seat %d exceeds capacity %d of table %s", *p.SeatNum, table.Capacity, table.Name)
	}
	return nil
}

func applySyncPayload(g *models.GuestRecord, p *SyncPayload, gid *uuid.UUID) {
	if p.Name != "" {
		g.Name = p.Name
	}
	if p.has("email") {
		g.Email = p.Email
	}
	if p.Pax >= 1 {
		g.Pax = p.Pax
	}
	if p.RSVP != "" {
		g.RSVP = p.RSVP
	}
	if p.has("isVip") {
		g.IsVip = p.IsVip
	}
	if p.has("notes") {
		g.Notes = p.Notes
	}
	if p.has("dietary") {
		g.Dietary = p.Dietary
	}
	if p.has("angbaoAmt") {
		g.AngbaoAmt = p.AngbaoAmt
	}
	if p.has("giftItem") {
		g.GiftItem = p.GiftItem
	}
	if p.has("tableId") {
		if p.TableID == nil || *p.TableID == "" {
			// JSON null and empty string both mean "unassign": clear the whole
			// seat assignment. SeatNum must never survive with a nil TableID.
			g.TableID = nil
			g.SeatNum = nil
		} else if tid, err := parseSyncID(*p.TableID); err == nil {
			g.TableID = &tid
			if p.has("seatNum") {
				g.SeatNum = p.SeatNum
			}
		}
	}
	if gid != nil {
		g.ID = *gid
	}
}

// appendNote joins existing notes and notes added at check-in with a newline.
func appendNote(existing, notes string) string {
	if notes == "" {
		return existing
	}
	if existing == "" {
		return notes
	}
	return existing + "\n" + notes
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
