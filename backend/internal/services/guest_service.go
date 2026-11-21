package services

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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

// ErrGuestCapExceeded is returned when adding guests would cross the cap.
var ErrGuestCapExceeded = errors.New("guest limit exceeded")

func guestCapError(current, delta int) error {
	return fmt.Errorf("%w: this wedding has %d guests; this change would make %d, over the %d-guest limit",
		ErrGuestCapExceeded, current, current+delta, MaxGuestsPerWedding)
}

// MaxGuestPartySize caps one guest entry's pax. Without it a single row could
// exceed the wedding cap or a table's whole seat range.
const MaxGuestPartySize = 100

// ErrInvalidPax is returned when a guest's pax is outside 1..MaxGuestPartySize.
var ErrInvalidPax = errors.New("invalid pax")

func validatePax(pax int) error {
	if pax < 1 || pax > MaxGuestPartySize {
		return fmt.Errorf("%w: pax must be between 1 and %d, got %d", ErrInvalidPax, MaxGuestPartySize, pax)
	}
	return nil
}

// Field caps keep one guest row from carrying unbounded text or a negative
// gift amount into storage and the public kiosk payload.
const (
	MaxGuestNameLen        = 200
	MaxGuestEmailLen       = 255 // the email column is varchar(255)
	MaxGuestNotesLen       = 2000
	MaxGuestGiftItemLen    = 200
	MaxGuestDietaryItems   = 20
	MaxGuestDietaryItemLen = 100
)

// ErrInvalidGuestField is returned when a guest text field is over its cap or
// the angbao amount is negative.
var ErrInvalidGuestField = errors.New("invalid guest field")

// ValidRSVP lists the accepted RSVP statuses; empty means "not set".
var ValidRSVP = map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}

func validateGuestFields(g *models.GuestRecord) error {
	if g.RSVP != "" && !ValidRSVP[g.RSVP] {
		return fmt.Errorf("%w: invalid RSVP status %q", ErrInvalidGuestField, g.RSVP)
	}
	if n := utf8.RuneCountInString(g.Name); n > MaxGuestNameLen {
		return fmt.Errorf("%w: name must be at most %d characters, got %d", ErrInvalidGuestField, MaxGuestNameLen, n)
	}
	if n := utf8.RuneCountInString(g.Email); n > MaxGuestEmailLen {
		return fmt.Errorf("%w: email must be at most %d characters", ErrInvalidGuestField, MaxGuestEmailLen)
	}
	if n := utf8.RuneCountInString(g.Notes); n > MaxGuestNotesLen {
		return fmt.Errorf("%w: notes must be at most %d characters, got %d", ErrInvalidGuestField, MaxGuestNotesLen, n)
	}
	if g.GiftItem != nil && utf8.RuneCountInString(*g.GiftItem) > MaxGuestGiftItemLen {
		return fmt.Errorf("%w: gift item must be at most %d characters", ErrInvalidGuestField, MaxGuestGiftItemLen)
	}
	if len(g.Dietary) > MaxGuestDietaryItems {
		return fmt.Errorf("%w: at most %d dietary entries", ErrInvalidGuestField, MaxGuestDietaryItems)
	}
	for _, d := range g.Dietary {
		if utf8.RuneCountInString(d) > MaxGuestDietaryItemLen {
			return fmt.Errorf("%w: each dietary entry must be at most %d characters", ErrInvalidGuestField, MaxGuestDietaryItemLen)
		}
	}
	if g.AngbaoAmt != nil && *g.AngbaoAmt < 0 {
		return fmt.Errorf("%w: angbao amount cannot be negative", ErrInvalidGuestField)
	}
	return nil
}

// guestCountLocked serializes guest additions for one wedding and returns the
// current count. The row lock is Postgres-only; sqlite (tests) has one writer.
func guestCountLocked(tx *gorm.DB, weddingID uuid.UUID) (int, error) {
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

func (s *GuestService) createGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord) error {
	if err := validateGuestFields(g); err != nil {
		return err
	}
	if g.Pax < 1 {
		g.Pax = 1
	}
	if err := validatePax(g.Pax); err != nil {
		return err
	}
	// Wedding row first, table row second: every write path locks in this
	// order, so the two lock sets cannot deadlock.
	total, err := guestCountLocked(tx, g.WeddingID)
	if err != nil {
		return err
	}
	if err := checkSeatNotTaken(ctx, tx, g); err != nil {
		return err
	}
	if total+g.Pax > MaxGuestsPerWedding {
		return guestCapError(total, g.Pax)
	}
	return repository.NewGuestRepo(tx).Create(ctx, g)
}

// updateGuestTx writes a full guest row. A non-nil base is the updated_at the
// editor loaded: if the row has changed since, the write is refused with
// ErrStaleGuest so it cannot silently undo someone else's change.
func (s *GuestService) updateGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord, syncWrite bool, base *time.Time) error {
	if err := validateGuestFields(g); err != nil {
		return err
	}
	if g.Pax < 1 {
		g.Pax = 1
	}
	if err := validatePax(g.Pax); err != nil {
		return err
	}
	total, err := guestCountLocked(tx, g.WeddingID)
	if err != nil {
		return err
	}
	if err := checkSeatNotTaken(ctx, tx, g); err != nil {
		return err
	}
	var current models.GuestRecord
	q := tx.Select("id", "pax", "updated_at").Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID)
	if tx.Name() == "postgres" {
		// Row lock so a check-in cannot commit between this read and the write.
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&current).Error; err != nil {
		return err
	}
	if base != nil && !current.UpdatedAt.Equal(*base) {
		return ErrStaleGuest
	}
	if g.Pax > current.Pax && total-current.Pax+g.Pax > MaxGuestsPerWedding {
		return guestCapError(total, g.Pax-current.Pax)
	}
	repo := repository.NewGuestRepo(tx)
	if syncWrite {
		return repo.SyncUpdate(ctx, g)
	}
	return repo.Update(ctx, g)
}

func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(ctx, weddingID, cursor, limit)
}

func (s *GuestService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(ctx, id, weddingID)
}

func (s *GuestService) Create(ctx context.Context, g *models.GuestRecord) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.createGuestTx(ctx, tx, g)
	})
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("create", g, g.WeddingID)
	return nil
}

func (s *GuestService) Update(ctx context.Context, g *models.GuestRecord) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.updateGuestTx(ctx, tx, g, false, nil)
	})
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("update", g, g.WeddingID)
	return nil
}

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Fetch guest before deletion so we can broadcast the event.
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	if err := s.guestRepo.Delete(ctx, id, weddingID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	s.sseHub.publishGuest("delete", guest, weddingID)
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
	table, err := lockTableTx(ctx, tx, weddingID, tableID)
	if err != nil {
		return err
	}
	if pax < 1 {
		return errors.New("guest pax must be at least 1")
	}
	// Compare against capacity-pax+1 so seatNum+pax cannot overflow past the
	// check for large seatNum values.
	if seatNum < 1 || seatNum > table.Capacity-pax+1 {
		return ErrSeatRange
	}
	existing, err := repository.NewGuestRepo(tx).FindByTable(ctx, weddingID, tableID)
	if err != nil {
		return err
	}
	guestEnd := seatNum + pax - 1
	// Seatless guests at the table take room too: count every party, so the
	// seat path and the table-only path in checkSeatNotTaken share one limit.
	atTable := pax
	for _, e := range existing {
		if e.ID == guestID {
			continue
		}
		atTable += e.Pax
		if e.SeatNum == nil {
			continue
		}
		eEnd := *e.SeatNum + e.Pax - 1
		if seatNum <= eEnd && *e.SeatNum <= guestEnd {
			return fmt.Errorf("%w: \"%s\" (seats %d-%d)", ErrSeatOverlap, e.Name, *e.SeatNum, eEnd)
		}
	}
	if atTable > table.Capacity {
		return ErrTableFull
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
		// Table without a seat: no seat range to check, but the party must still
		// fit, under the same table lock as seat writes.
		table, err := lockTableTx(ctx, tx, g.WeddingID, *g.TableID)
		if err != nil {
			return err
		}
		var seated int64
		if err := tx.Model(&models.GuestRecord{}).Select("COALESCE(SUM(pax), 0)").
			Where("wedding_id = ? AND table_id = ? AND id <> ?", g.WeddingID, *g.TableID, g.ID).
			Scan(&seated).Error; err != nil {
			return fmt.Errorf("sum table pax: %w", err)
		}
		if int(seated)+g.Pax > table.Capacity {
			return ErrTableFull
		}
		return nil
	}
	return checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, g.ID, g.Pax, *g.SeatNum)
}

// lockTableTx row-locks the table on Postgres (sqlite tests have one writer)
// and returns it, so every seat/capacity check serializes on the same row.
func lockTableTx(ctx context.Context, tx *gorm.DB, weddingID, tableID uuid.UUID) (*models.BanquetTable, error) {
	if tx.Name() == "postgres" {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
			Where("id = ? AND wedding_id = ?", tableID, weddingID).First(&models.BanquetTable{}).Error; err != nil {
			return nil, tableLookupErr(err)
		}
	}
	table, err := repository.NewTableRepo(tx).FindByID(ctx, tableID, weddingID)
	if err != nil {
		return nil, tableLookupErr(err)
	}
	return table, nil
}

// tableLookupErr reports a missing row as ErrTableNotFound and passes any other
// error through: a dropped connection is retryable, not a missing table.
func tableLookupErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTableNotFound
	}
	return fmt.Errorf("lock table: %w", err)
}

func (s *GuestService) AssignSeat(ctx context.Context, guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	var guest *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGuestRepo := repository.NewGuestRepo(tx)
		// Wedding row lock keeps the lock order identical to create/update, so
		// a concurrent seat move and import cannot deadlock each other.
		if _, err := guestCountLocked(tx, weddingID); err != nil {
			return err
		}
		// Read the guest after the lock: a party-size edit committed while we
		// waited must count, or the seat check uses the old pax and overbooks.
		var err error
		guest, err = txGuestRepo.FindByID(ctx, guestID, weddingID)
		if err != nil {
			return err
		}
		if err := checkSeatTx(ctx, tx, weddingID, tableID, guestID, guest.Pax, seatNum); err != nil {
			return err
		}
		guest.TableID = &tableID
		guest.SeatNum = &seatNum
		return txGuestRepo.SetSeat(ctx, guestID, weddingID, guest.TableID, guest.SeatNum)
	})
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("seat_assign", guest, weddingID)
	return nil
}

// UnassignSeat clears only the seat columns, so a concurrent check-in or notes
// edit is not overwritten by a stale full-row write.
func (s *GuestService) UnassignSeat(ctx context.Context, guestID, weddingID uuid.UUID) error {
	if err := s.guestRepo.SetSeat(ctx, guestID, weddingID, nil, nil); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	guest, err := s.guestRepo.FindByID(ctx, guestID, weddingID)
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("update", guest, weddingID)
	return nil
}

// UpdateWithSeat saves edited fields and, when tableID is set, moves the guest
// to that table in the same transaction — a failed seat check leaves the
// fields untouched and a failed field write leaves the seat where it was.
// A nil tableID preserves the current seat assignment. A non-nil base refuses
// the write with ErrStaleGuest if the row changed after the editor loaded it.
func (s *GuestService) UpdateWithSeat(ctx context.Context, g *models.GuestRecord, tableID *uuid.UUID, seatNum *int, base *time.Time) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tableID != nil {
			g.TableID = tableID
			g.SeatNum = seatNum
		}
		return s.updateGuestTx(ctx, tx, g, false, base)
	})
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("update", g, g.WeddingID)
	return nil
}

// CheckIn marks a guest as checked in and promotes their RSVP to confirmed.
// Non-empty notes are appended to the guest's existing notes.
// Returns ErrAlreadyCheckedIn if the guest was already checked in by another
// receptionist (FIFO conflict).
func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID, notes string) error {
	now := time.Now()
	notes = strings.TrimSpace(notes)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updatedNotes := ""
		if notes != "" {
			// Read the notes under a row lock: a notes edit committing between
			// this read and the write below would otherwise be overwritten.
			var existing models.GuestRecord
			q := tx.Select("notes").Where("id = ? AND wedding_id = ?", id, weddingID)
			if tx.Name() == "postgres" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q.First(&existing).Error; err != nil {
				return err
			}
			updatedNotes = appendNote(existing.Notes, notes)
		}
		if utf8.RuneCountInString(updatedNotes) > MaxGuestNotesLen {
			return fmt.Errorf("%w: notes must be at most %d characters", ErrInvalidGuestField, MaxGuestNotesLen)
		}
		// Atomic conditional update: only check in if not already checked in
		return repository.NewGuestRepo(tx).ConditionalCheckIn(ctx, id, weddingID, now, updatedNotes)
	})
	if err != nil {
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
		s.sseHub.publishGuest("checkin", guest, weddingID)
	}
	return nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	if err := s.guestRepo.CheckOut(ctx, id, weddingID, time.Now()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("checkout", guest, weddingID)
	return nil
}

// BulkCreate imports guests in one transaction. rows[i] is the spreadsheet row
// guests[i] came from, so an error names the line to fix; without it, rows
// count from 1 in slice order.
func (s *GuestService) BulkCreate(ctx context.Context, guests []models.GuestRecord, rows []int) (int, error) {
	created := 0
	if len(guests) == 0 {
		return 0, nil
	}
	rowOf := func(i int) int {
		if i < len(rows) && rows[i] > 0 {
			return rows[i]
		}
		return i + 1
	}
	weddingID := guests[0].WeddingID
	incoming := 0
	for i := range guests {
		if guests[i].Pax < 1 {
			guests[i].Pax = 1
		}
		if err := validatePax(guests[i].Pax); err != nil {
			return 0, fmt.Errorf("row %d: %w", rowOf(i), err)
		}
		if err := validateGuestFields(&guests[i]); err != nil {
			return 0, fmt.Errorf("row %d: %w", rowOf(i), err)
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
		created = 0
		events = events[:0]
		total, err := guestCountLocked(tx, weddingID)
		if err != nil {
			return err
		}
		if total+incoming > MaxGuestsPerWedding {
			return guestCapError(total, incoming)
		}
		for i := range guests {
			g := &guests[i]
			if err := bulkCreateOne(ctx, tx, txGuestRepo, g); err != nil {
				// Name the CSV row, so an 800-row import says which one to fix.
				return fmt.Errorf("row %d: %w", rowOf(i), err)
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
		s.sseHub.publishGuest(e.eventType, e.guest, e.guest.WeddingID)
	}
	return created, nil
}

// bulkCreateOne checks g's table and seat inside tx, then creates it.
func bulkCreateOne(ctx context.Context, tx *gorm.DB, repo *repository.GuestRepo, g *models.GuestRecord) error {
	if g.Pax < 1 {
		g.Pax = 1
	}
	switch {
	case g.TableID == nil:
	case g.SeatNum == nil:
		// A table without a seat still checks ownership and party capacity.
		if err := checkSeatNotTaken(ctx, tx, g); err != nil {
			return fmt.Errorf("table %s: %w", g.TableID.String(), err)
		}
	default:
		if err := checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, uuid.Nil, g.Pax, *g.SeatNum); err != nil {
			return err
		}
	}
	return repo.Create(ctx, g)
}

func (s *GuestService) Occupancy(ctx context.Context, weddingID uuid.UUID) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(ctx, weddingID)
}

// publishGuest broadcasts a guest mutation to all connected SSE clients via Redis.
// It publishes inline, not in a goroutine: two events for one guest must reach
// clients in the order they were written, or an older state lands last.
// Uses context.Background() so a request cancelled after commit still
// publishes. A nil hub (tests, SSE disabled) is a no-op.
const publishTimeout = 500 * time.Millisecond

func (h *SSEHub) publishGuest(eventType string, guest *models.GuestRecord, weddingID uuid.UUID) {
	if h == nil {
		return
	}

	event := GuestEvent{
		Type:      eventType,
		GuestID:   guest.ID.String(),
		WeddingID: weddingID.String(),
		Guest:     guestToEventData(guest),
		Timestamp: time.Now().UnixMilli(),
	}

	// Bounded: the write already committed, and a stalled Redis must not hold
	// every check-in (and each item of a sync batch) for go-redis's retries.
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := h.Publish(ctx, weddingID, event); err != nil {
		log.Printf("SSE: publish %s for guest %s in wedding %s: %v", eventType, guest.ID, weddingID, err)
	}
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

// SyncPayload is a partial guest record from the offline queue. Pointer fields
// distinguish "absent" from a zero value, so a seat-only or notes-only mutation
// never erases the rest of the record. Fields a user can clear (table, angbao,
// gift) use NullableJSON so an explicit null clears them.
type SyncPayload struct {
	Name      *string              `json:"name"`
	Email     *string              `json:"email"`
	Pax       *int                 `json:"pax"`
	RSVP      *string              `json:"rsvp"`
	IsVip     *bool                `json:"isVip"`
	Notes     *string              `json:"notes"`
	Dietary   *[]string            `json:"dietary"`
	TableID   NullableJSON[string] `json:"tableId"`
	SeatNum   *int                 `json:"seatNum"`
	AngbaoAmt NullableJSON[int]    `json:"angbaoAmt"`
	GiftItem  NullableJSON[string] `json:"giftItem"`
}

// NullableJSON records whether a JSON field was present and whether it was null.
// A plain *T cannot tell an absent field from an explicit null, and sync needs
// that to tell "leave the seat alone" from "unassign".
type NullableJSON[T any] struct {
	Set   bool
	Value *T
}

func (n *NullableJSON[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
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
	// Retryable marks a failure that is not the mutation's fault (database
	// error, timeout). The client keeps it queued without spending a retry.
	Retryable bool `json:"retryable,omitzero"`
}

// syncRuleErrors are failures that retrying the same mutation cannot fix.
var syncRuleErrors = []error{
	ErrInvalidGuestField, ErrInvalidPax, ErrGuestCapExceeded, ErrSeatOverlap, ErrStaleGuest,
	ErrSeatRange, ErrTableFull, ErrTableNotFound, errInvalidTableID, gorm.ErrRecordNotFound,
}

// syncFailed reports err for m, marking anything that is not a rule
// violation as retryable.
func syncFailed(m SyncMutation, err error) SyncResult {
	rule := slices.ContainsFunc(syncRuleErrors, func(target error) bool { return errors.Is(err, target) })
	return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error(), Retryable: !rule}
}

// Sync applies a batch of mutations with newer-time-wins.
// Mutations are processed in order received, FIFO.
func (s *GuestService) Sync(ctx context.Context, weddingID uuid.UUID, mutations []SyncMutation) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(mutations))
	// Guests with a queued edit that did not apply. The client sends a later
	// edit of the same guest without a base: it was built on the refused edit,
	// so applying it would undo whatever newer change refused the first one.
	refused := make(map[string]bool)
	for _, m := range mutations {
		edit := m.Op == SyncOpUpdate || m.Op == SyncOpCreate
		var res SyncResult
		if edit && m.BaseUpdatedAt == nil && refused[m.GuestID] {
			res = syncFailed(m, ErrStaleGuest)
		} else {
			res = s.applySyncMutation(ctx, weddingID, m)
		}
		if edit && res.Status != "applied" {
			refused[m.GuestID] = true
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *GuestService) applySyncMutation(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	gid, err := utils.DecodeUUID(m.GuestID)
	if err != nil {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "invalid guestId"}
	}

	// Skew clamp: never trust a client clock in the future, or a skewed device
	// would win every LWW comparison from then on.
	opTime := m.ClientUpdatedAt
	if opTime.After(time.Now()) {
		opTime = time.Now()
	}

	// Load server record if exists. Only "no such row" means not found: any
	// other error must stay "failed", or the client dequeues the mutation.
	existing, err := s.guestRepo.FindByID(ctx, gid, weddingID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return syncFailed(m, fmt.Errorf("load guest: %w", err))
	}
	found := err == nil

	switch m.Op {
	case SyncOpCreate:
		if found {
			// treat as update with LWW
			if opTime.Before(existing.UpdatedAt) {
				return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
			}
			if opTime.Equal(existing.UpdatedAt) {
				// Replay of a write whose response was lost: it is already applied.
				return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}
			}
			if m.Payload != nil {
				if err := applySyncPayload(existing, m.Payload, &gid); err != nil {
					return syncFailed(m, err)
				}
			}
			if err := s.syncUpdate(ctx, existing, m, opTime); err != nil {
				return syncFailed(m, err)
			}
			s.sseHub.publishGuest("update", existing, weddingID)
			return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}
		}
		if m.Payload == nil || m.Payload.Name == nil || *m.Payload.Name == "" {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "name required"}
		}
		pax := 1
		if m.Payload.Pax != nil && *m.Payload.Pax >= 1 {
			pax = *m.Payload.Pax
		}
		g := &models.GuestRecord{
			ID:        gid,
			WeddingID: weddingID,
			Name:      *m.Payload.Name,
			Pax:       pax,
			AngbaoAmt: m.Payload.AngbaoAmt.Value,
			GiftItem:  m.Payload.GiftItem.Value,
			CreatedAt: opTime,
			UpdatedAt: opTime,
		}
		if m.Payload.Email != nil {
			g.Email = *m.Payload.Email
		}
		if m.Payload.RSVP != nil {
			g.RSVP = *m.Payload.RSVP
		}
		if m.Payload.IsVip != nil {
			g.IsVip = *m.Payload.IsVip
		}
		if m.Payload.Notes != nil {
			g.Notes = *m.Payload.Notes
		}
		if m.Payload.Dietary != nil {
			g.Dietary = *m.Payload.Dietary
		}
		if t := m.Payload.TableID; t.Set && t.Value != nil && *t.Value != "" {
			tid, err := utils.DecodeUUID(*t.Value)
			if err != nil {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: fmt.Sprintf("invalid tableId %q", *t.Value)}
			}
			g.TableID = &tid
		}
		if g.TableID != nil {
			g.SeatNum = m.Payload.SeatNum
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.createGuestTx(ctx, tx, g)
		}); err != nil {
			return syncFailed(m, err)
		}
		s.sseHub.publishGuest("create", g, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}

	case SyncOpUpdate:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found", ServerRecord: nil}
		}
		if opTime.Before(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
		}
		// The row already carries this op's time: a replay of a write whose
		// response was lost. Re-applying it would fail its now-stale base.
		if opTime.Equal(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}
		}
		if m.Payload != nil {
			if err := applySyncPayload(existing, m.Payload, nil); err != nil {
				return syncFailed(m, err)
			}
		}
		if err := s.syncUpdate(ctx, existing, m, opTime); err != nil {
			return syncFailed(m, err)
		}
		s.sseHub.publishGuest("update", existing, weddingID)
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
			return syncFailed(m, err)
		}
		s.sseHub.publishGuest("delete", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied"}

	case SyncOpCheckIn:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		var res SyncResult
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Reload under a row lock: two phones syncing the same arrival at
			// once must each see the other's write, or the last one overwrites
			// the first angbao and note.
			q := tx.Where("id = ? AND wedding_id = ?", gid, weddingID)
			if tx.Name() == "postgres" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q.First(existing).Error; err != nil {
				return err
			}
			var err error
			res, err = syncCheckIn(ctx, tx, existing, m, opTime)
			return err
		}); err != nil {
			return syncFailed(m, err)
		}
		if res.Status == "applied" {
			s.sseHub.publishGuest("checkin", existing, weddingID)
		}
		return res

	case SyncOpCheckOut:
		if !found {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if opTime.Before(existing.UpdatedAt) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: existing}
		}
		existing.CheckedInAt = nil
		existing.UpdatedAt = opTime
		if err := s.guestRepo.CheckOut(ctx, gid, weddingID, opTime); err != nil {
			return syncFailed(m, err)
		}
		s.sseHub.publishGuest("checkout", existing, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: existing}

	default:
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "unknown op"}
	}
}

// errGuestChangedDuringSync means the guest changed between this sync's own
// read and its write. Not in syncRuleErrors: a retry reloads and succeeds.
var errGuestChangedDuringSync = errors.New("guest changed during sync")

// syncUpdate writes a queued edit of g. The client's base (the updatedAt its
// editor loaded) refuses edits made on a stale copy, like the online 409; that
// is a rule error. With no client base, the check still guards the gap since
// this sync loaded g.
func (s *GuestService) syncUpdate(ctx context.Context, g *models.GuestRecord, m SyncMutation, opTime time.Time) error {
	base := cmp.Or(m.BaseUpdatedAt, new(g.UpdatedAt))
	g.UpdatedAt = opTime
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.updateGuestTx(ctx, tx, g, true, base)
	})
	if m.BaseUpdatedAt == nil && errors.Is(err, ErrStaleGuest) {
		return errGuestChangedDuringSync
	}
	return err
}

// syncCheckIn applies a queued check-in to g, which the caller loaded under a
// row lock inside tx. A skip returns a result and no error; an error rolls back.
func syncCheckIn(ctx context.Context, tx *gorm.DB, g *models.GuestRecord, m SyncMutation, opTime time.Time) (SyncResult, error) {
	// An arrival recorded offline wins over unrelated newer edits (table
	// moves, notes): skipping it would lose the check-in and angbao.
	if g.CheckedInAt != nil && opTime.Before(g.UpdatedAt) {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: g}, nil
	}
	wasCheckedIn := g.CheckedInAt != nil
	if wasCheckedIn && opTime.Equal(g.UpdatedAt) {
		// Replay of this check-in whose response was lost: already applied.
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}, nil
	}
	// A second check-in (another receptionist got there first) keeps the
	// first arrival time and any angbao/gift already keyed, like the online
	// path's 409: only fill what is still empty and append the note.
	if !wasCheckedIn {
		g.CheckedInAt = &opTime
	}
	g.RSVP = "confirmed"
	if m.Payload != nil {
		if v := m.Payload.AngbaoAmt.Value; v != nil && (!wasCheckedIn || g.AngbaoAmt == nil) {
			g.AngbaoAmt = v
		}
		if v := m.Payload.GiftItem.Value; v != nil && (!wasCheckedIn || g.GiftItem == nil) {
			g.GiftItem = v
		}
		// A notes value in a checkin payload is appended to the guest's notes,
		// never replacing them, so a queued note never wipes existing notes.
		if m.Payload.Notes != nil {
			// A retried checkin whose response was lost finds the guest checked
			// in with its note already present: skip it, don't duplicate it.
			note := strings.TrimSpace(*m.Payload.Notes)
			if !wasCheckedIn || !slices.Contains(strings.Split(g.Notes, "\n"), note) {
				g.Notes = appendNote(g.Notes, note)
			}
		}
	}
	if err := validateGuestFields(g); err != nil {
		return SyncResult{}, err
	}
	// Never move updated_at backwards, or a stale queued edit could win next.
	// Always move it forward, though: an editor holding the old updated_at
	// must get a 409, not silently overwrite this check-in's angbao and note.
	if opTime.After(g.UpdatedAt) {
		g.UpdatedAt = opTime
	} else {
		g.UpdatedAt = g.UpdatedAt.Add(time.Microsecond)
	}
	if err := repository.NewGuestRepo(tx).SyncCheckIn(ctx, g); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}, nil
}

func applySyncPayload(g *models.GuestRecord, p *SyncPayload, gid *uuid.UUID) error {
	if p.Name != nil && *p.Name != "" {
		g.Name = *p.Name
	}
	if p.Email != nil {
		g.Email = *p.Email
	}
	if p.Pax != nil && *p.Pax >= 1 {
		g.Pax = *p.Pax
	}
	if p.RSVP != nil && *p.RSVP != "" {
		g.RSVP = *p.RSVP
	}
	if p.IsVip != nil {
		g.IsVip = *p.IsVip
	}
	if p.Notes != nil {
		g.Notes = *p.Notes
	}
	if p.Dietary != nil {
		g.Dietary = *p.Dietary
	}
	// Present-but-null clears the value; absent leaves it alone.
	if p.AngbaoAmt.Set {
		g.AngbaoAmt = p.AngbaoAmt.Value
	}
	if p.GiftItem.Set {
		g.GiftItem = p.GiftItem.Value
	}
	// Only an explicitly sent tableId touches the seat assignment: absent means
	// "leave it alone", null or empty means unassign.
	if p.TableID.Set {
		if p.TableID.Value == nil || *p.TableID.Value == "" {
			g.TableID = nil
			g.SeatNum = nil
		} else {
			tid, err := utils.DecodeUUID(*p.TableID.Value)
			if err != nil {
				return fmt.Errorf("%w %q", errInvalidTableID, *p.TableID.Value)
			}
			g.TableID = &tid
			g.SeatNum = p.SeatNum
		}
	}
	if gid != nil {
		g.ID = *gid
	}
	return nil
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

func guestToEventData(g *models.GuestRecord) *GuestEventData {
	d := &GuestEventData{
		ID:      utils.EncodeUUID(g.ID),
		Name:    g.Name,
		Pinyin:  g.NamePinyin,
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
