package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"reflect"
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
// current count.
func guestCountLocked(tx *gorm.DB, weddingID uuid.UUID) (int, error) {
	if err := lockWeddingTx(tx, weddingID); err != nil {
		return 0, err
	}
	var total int
	if err := tx.Model(&models.GuestRecord{}).
		Where("wedding_id = ?", weddingID).
		Select("COALESCE(SUM(pax), 0)").Row().Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// lockWeddingTx takes the wedding row lock every guest write takes first, so a
// sync merge and an online edit of the same wedding serialize in one order.
// The lock is Postgres-only; sqlite (tests) has one writer.
func lockWeddingTx(tx *gorm.DB, weddingID uuid.UUID) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
		Where("id = ?", weddingID).First(&models.WeddingEvent{}).Error
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
	q := tx.Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID)
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
	// An edit carries no checked_in_at: keep the committed arrival, so the
	// broadcast after the write cannot report it cleared.
	g.CheckedInAt = current.CheckedInAt
	if g.Pax > current.Pax && total-current.Pax+g.Pax > MaxGuestsPerWedding {
		return guestCapError(total, g.Pax-current.Pax)
	}
	repo := repository.NewGuestRepo(tx)
	if syncWrite {
		return repo.SyncUpdate(ctx, g)
	}
	// An online write carries every edit field with server time, so a queued
	// offline edit of any of them loses to it.
	g.StampFields(time.Now(), models.GuestEditFields)
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
// A nil tableID keeps the current seat assignment and uuid.Nil clears it. A
// non-nil base refuses the write with ErrStaleGuest if the row changed after
// the editor loaded it.
func (s *GuestService) UpdateWithSeat(ctx context.Context, g *models.GuestRecord, tableID *uuid.UUID, seatNum *int, base *time.Time) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		switch {
		case tableID == nil:
			// Keep the seat as committed, not as the caller read it before this
			// transaction: a seat move in between must survive. Wedding lock
			// first keeps the lock order of every other guest write.
			if _, err := guestCountLocked(tx, g.WeddingID); err != nil {
				return err
			}
			var cur models.GuestRecord
			q := tx.Select("table_id", "seat_num").Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID)
			if tx.Name() == "postgres" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q.First(&cur).Error; err != nil {
				return err
			}
			g.TableID, g.SeatNum = cur.TableID, cur.SeatNum
		case *tableID == uuid.Nil:
			g.TableID, g.SeatNum = nil, nil
		default:
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
		// Read the row under a row lock: a notes edit committing between this read
		// and the write below would otherwise be overwritten, and the check-in
		// stamps the fields it writes with this write's time.
		var existing models.GuestRecord
		q := tx.Where("id = ? AND wedding_id = ?", id, weddingID)
		if tx.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&existing).Error; err != nil {
			return err
		}
		updatedNotes := ""
		if notes != "" {
			updatedNotes = appendNote(existing.Notes, notes)
			existing.StampField(models.FieldNotes, now)
		}
		if utf8.RuneCountInString(updatedNotes) > MaxGuestNotesLen {
			return fmt.Errorf("%w: notes must be at most %d characters", ErrInvalidGuestField, MaxGuestNotesLen)
		}
		existing.StampField(models.FieldCheckedIn, now)
		existing.StampField(models.FieldRSVP, now)
		// Atomic conditional update: only check in if not already checked in
		return repository.NewGuestRepo(tx).ConditionalCheckIn(ctx, id, weddingID, now, updatedNotes, existing.FieldUpdatedAt)
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
		// No event to send: make this wedding's streams reload instead.
		log.Printf("guest lookup after check-in failed: %v", err)
		s.sseHub.resync(weddingID)
		return nil
	}
	s.sseHub.publishGuest("checkin", guest, weddingID)
	return nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	for attempt := 0; ; attempt++ {
		err := s.guestRepo.CheckOut(ctx, id, weddingID, time.Now())
		if err == nil {
			break
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Either no such guest, or one who is not checked in: the latter is
		// already in the wanted state, so succeed without a write or event.
		g, err := s.guestRepo.FindByID(ctx, id, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		if err != nil {
			return err
		}
		if g.CheckedInAt == nil {
			return nil
		}
		// A check-in committed between the update and the re-read: retry, so
		// success is never reported while the guest is still checked in.
		if attempt == 2 {
			return errGuestChangedDuringSync
		}
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

// publishPause is how long publishes are skipped after one times out, so a
// stalled Redis costs one publishTimeout per pause instead of one per row.
// Events skipped meanwhile would have timed out anyway.
const publishPause = 2 * time.Second

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
	if time.Now().UnixNano() < h.pausedUntil.Load() {
		log.Printf("SSE: publish %s for guest %s in wedding %s skipped: Redis paused after failure", eventType, guest.ID, weddingID)
		h.resync(weddingID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := h.Publish(ctx, weddingID, event); err != nil {
		// Only a stall pauses: other failures (hub not ready yet, a bad
		// event) cost no time and must not drop the events after them.
		if ne, ok := errors.AsType[net.Error](err); errors.Is(err, context.DeadlineExceeded) || (ok && ne.Timeout()) {
			h.pausedUntil.Store(time.Now().Add(publishPause).UnixNano())
		}
		log.Printf("SSE: publish %s for guest %s in wedding %s: %v", eventType, guest.ID, weddingID, err)
		h.resync(weddingID)
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

	// decodeErr is set when this element could not be decoded, so one
	// malformed mutation fails alone instead of the whole batch.
	decodeErr error
}

// UnmarshalJSON keeps a malformed mutation from failing its whole batch: the
// error is reported in that mutation's own result.
func (m *SyncMutation) UnmarshalJSON(b []byte) error {
	type alias SyncMutation
	if err := json.Unmarshal(b, (*alias)(m)); err != nil {
		m.decodeErr = err
	}
	return nil
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
	gorm.ErrDuplicatedKey,
}

// syncFailed reports err for m, marking anything that is not a rule
// violation as retryable.
func syncFailed(m SyncMutation, err error) SyncResult {
	rule := slices.ContainsFunc(syncRuleErrors, func(target error) bool { return errors.Is(err, target) })
	if rule {
		return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error()}
	}
	// Infrastructure errors carry hostnames, SQL and driver detail: log them
	// and send the client a generic reason, like internalErrorHandler does.
	slog.Error("sync mutation failed", "guest", m.GuestID, "op", m.Op, "err", err)
	return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "server error, will retry", Retryable: true}
}

// Sync applies a batch of mutations field by field: each field keeps its
// newest write. Mutations are processed in order received, FIFO.
func (s *GuestService) Sync(ctx context.Context, weddingID uuid.UUID, mutations []SyncMutation) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(mutations))
	// Guests whose create failed retryably (server outage). Their later ops
	// wait with it: run now, a check-in finds no row, is "skipped" and
	// dequeued, and the retried create comes back without the arrival.
	createPending := make(map[string]bool)
	for _, m := range mutations {
		res := SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "waiting for guest create", Retryable: true}
		if !createPending[m.GuestID] {
			res = s.applySyncMutation(ctx, weddingID, m)
		}
		if m.Op == SyncOpCreate && res.Retryable {
			createPending[m.GuestID] = true
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *GuestService) applySyncMutation(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	if m.decodeErr != nil {
		return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "malformed mutation: " + m.decodeErr.Error()}
	}
	gid, err := utils.DecodeUUID(m.GuestID)
	if err != nil {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "invalid guestId"}
	}
	// Skew clamp: never trust a client clock in the future, or a skewed device
	// would win every LWW comparison from then on.
	// A missing clientUpdatedAt would stamp check-ins as year 1 and make
	// every edit lose to the server: refuse it as a rule error.
	if m.ClientUpdatedAt.IsZero() {
		return syncFailed(m, fmt.Errorf("%w: clientUpdatedAt is required", ErrInvalidGuestField))
	}
	opTime := m.ClientUpdatedAt
	if opTime.After(time.Now()) {
		opTime = time.Now()
	}

	switch m.Op {
	case SyncOpCreate:
		return s.syncCreate(ctx, weddingID, gid, m, opTime)
	case SyncOpUpdate:
		return s.syncEdit(ctx, weddingID, gid, m, opTime)
	case SyncOpDelete:
		return s.syncDelete(ctx, weddingID, gid, m, opTime)
	case SyncOpCheckIn:
		return s.syncCheckIn(ctx, weddingID, gid, m, opTime)
	case SyncOpCheckOut:
		return s.syncCheckOut(ctx, weddingID, gid, m, opTime)
	default:
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "unknown op"}
	}
}

// errGuestChangedDuringSync means the guest changed between a read and its
// write. Not in syncRuleErrors: a retry reloads and succeeds.
var errGuestChangedDuringSync = errors.New("guest changed during sync")

// syncMerge records how a mutation's fields fared against the row they merged
// into: changed means a value differs, won and lost say whether a carried field
// was newer or older than that field's own last write.
type syncMerge struct {
	changed bool
	won     bool
	lost    bool
}

// take reports whether opTime wins field. A field written at the same instant
// already holds this mutation's value (a replay), so it takes no write.
func (r *syncMerge) take(g *models.GuestRecord, field string, opTime time.Time) bool {
	switch written := g.WrittenAt(field); {
	case opTime.After(written):
		r.won = true
		return true
	case opTime.Before(written):
		r.lost = true
	}
	return false
}

// result reports the mutation's outcome: a row it changed is applied, a
// mutation whose every field lost to a newer server write is skipped, and a
// replay that changed nothing is applied (invariant 5).
func (r syncMerge) result(m SyncMutation, g *models.GuestRecord) SyncResult {
	if !r.changed && r.lost && !r.won {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: g}
	}
	return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}
}

// nextWriteTime keeps updated_at moving forward: an editor holding the old value
// must get a 409, never silently overwrite this write.
func nextWriteTime(row, op time.Time) time.Time {
	if op.After(row) {
		return op
	}
	return row.Add(time.Microsecond)
}

// newerThanEveryField reports whether op is newer than every field write on g,
// which is when a delete applies (invariant 4).
func newerThanEveryField(g *models.GuestRecord, op time.Time) bool {
	if op.Before(g.WrittenAt(models.FieldCheckedIn)) {
		return false
	}
	for _, field := range models.GuestEditFields {
		if op.Before(g.WrittenAt(field)) {
			return false
		}
	}
	return true
}

// syncEdit merges a queued edit into the row and writes the fields that won,
// under one transaction: a write committing between the read and the write
// cannot be undone unseen.
func (s *GuestService) syncEdit(ctx context.Context, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time) SyncResult {
	if err := validateSyncPayload(m.Payload); err != nil {
		return syncFailed(m, err)
	}
	var res SyncResult
	var wrote bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Wedding row first, as every guest write locks in that order.
		if err := lockWeddingTx(tx, weddingID); err != nil {
			return err
		}
		repo := repository.NewGuestRepo(tx)
		g, err := repo.FindByIDLocked(ctx, gid, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res = SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
			return nil
		}
		if err != nil {
			return err
		}
		merge, err := mergeSyncPayload(g, m.Payload, opTime)
		if err != nil {
			return err
		}
		res = merge.result(m, g)
		if !merge.changed {
			return nil
		}
		g.UpdatedAt = nextWriteTime(g.UpdatedAt, opTime)
		if err := s.updateGuestTx(ctx, tx, g, true, nil); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	if err != nil {
		return syncFailed(m, err)
	}
	if wrote {
		s.sseHub.publishGuest("update", res.ServerRecord, weddingID)
	}
	return res
}

// syncCreate creates a guest a phone added offline. An id that already exists
// merges like an edit, so a replayed create cannot undo a newer write.
func (s *GuestService) syncCreate(ctx context.Context, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time) SyncResult {
	if m.Payload == nil || m.Payload.Name == nil || strings.TrimSpace(*m.Payload.Name) == "" {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "name required"}
	}
	if err := validateSyncPayload(m.Payload); err != nil {
		return syncFailed(m, err)
	}
	var res SyncResult
	var event string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Wedding row first, as every guest write locks in that order.
		if err := lockWeddingTx(tx, weddingID); err != nil {
			return err
		}
		repo := repository.NewGuestRepo(tx)
		g, err := repo.FindByIDLocked(ctx, gid, weddingID)
		switch {
		case err == nil:
			merge, err := mergeSyncPayload(g, m.Payload, opTime)
			if err != nil {
				return err
			}
			res = merge.result(m, g)
			if !merge.changed {
				return nil
			}
			g.UpdatedAt = nextWriteTime(g.UpdatedAt, opTime)
			if err := s.updateGuestTx(ctx, tx, g, true, nil); err != nil {
				return err
			}
			event = "update"
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		// A replayed create (first response lost) must not bring back a guest
		// someone deleted since.
		deleted, err := repo.IsDeleted(ctx, gid, weddingID)
		if err != nil {
			return err
		}
		if deleted {
			res = SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "deleted"}
			return nil
		}
		g, err = newSyncGuest(weddingID, gid, m.Payload, opTime)
		if err != nil {
			return err
		}
		if err := s.createGuestTx(ctx, tx, g); err != nil {
			return err
		}
		res = SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}
		event = "create"
		return nil
	})
	if err != nil {
		// A guestId already taken (say, by another wedding's guest) stays
		// taken: report the key collision as a rule error, not retryable.
		if t, ok := s.db.Dialector.(gorm.ErrorTranslator); ok {
			err = t.Translate(err)
		}
		return syncFailed(m, err)
	}
	if event != "" {
		s.sseHub.publishGuest(event, res.ServerRecord, weddingID)
	}
	return res
}

// syncDelete deletes the guest when the delete is newer than every field write
// on the row (invariant 4). An older delete fails so the client keeps it queued.
func (s *GuestService) syncDelete(ctx context.Context, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time) SyncResult {
	var res SyncResult
	var row *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		g, err := repo.FindByIDLocked(ctx, gid, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Already gone: the intent holds.
			res = SyncResult{GuestID: m.GuestID, Status: "applied"}
			return nil
		}
		if err != nil {
			return err
		}
		row = g
		if !newerThanEveryField(g, opTime) {
			// Report "failed", not "skipped": the client keeps failed items
			// queued instead of dequeuing and losing the delete intent.
			res = SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "older than server", ServerRecord: g}
			return nil
		}
		res = SyncResult{GuestID: m.GuestID, Status: "applied"}
		return repo.Delete(ctx, gid, weddingID)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Someone else deleted it first: the intent holds.
			return SyncResult{GuestID: m.GuestID, Status: "applied"}
		}
		return syncFailed(m, err)
	}
	if res.Status == "applied" && row != nil {
		s.sseHub.publishGuest("delete", row, weddingID)
	}
	return res
}

// syncCheckIn applies a queued check-in field by field: the arrival, the RSVP,
// the angbao and gift it carries, and its note appended (invariant 3).
func (s *GuestService) syncCheckIn(ctx context.Context, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time) SyncResult {
	var res SyncResult
	var wrote bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		g, err := repo.FindByIDLocked(ctx, gid, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res = SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
			return nil
		}
		if err != nil {
			return err
		}
		merge, err := mergeSyncCheckIn(g, m.Payload, opTime)
		if err != nil {
			return err
		}
		res = merge.result(m, g)
		if !merge.changed {
			return nil
		}
		if err := validateGuestFields(g); err != nil {
			return err
		}
		g.UpdatedAt = nextWriteTime(g.UpdatedAt, opTime)
		if err := repo.SyncCheckIn(ctx, g); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	if err != nil {
		return syncFailed(m, err)
	}
	if wrote {
		s.sseHub.publishGuest("checkin", res.ServerRecord, weddingID)
	}
	return res
}

// syncCheckOut applies a queued check-out: it clears the arrival when the undo
// is newer than the row's own write of checked_in_at (invariant 2). An undo
// behind a newer check-in skips; an undo of a guest already checked out is
// applied.
func (s *GuestService) syncCheckOut(ctx context.Context, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time) SyncResult {
	var res SyncResult
	var wrote bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		g, err := repo.FindByIDLocked(ctx, gid, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res = SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
			return nil
		}
		if err != nil {
			return err
		}
		var merge syncMerge
		if merge.take(g, models.FieldCheckedIn, opTime) {
			merge.changed = g.CheckedInAt != nil
			g.CheckedInAt = nil
			g.StampField(models.FieldCheckedIn, opTime)
		}
		res = merge.result(m, g)
		if !merge.changed {
			return nil
		}
		g.UpdatedAt = nextWriteTime(g.UpdatedAt, opTime)
		if err := repo.CheckOut(ctx, gid, weddingID, g.UpdatedAt); err != nil {
			return err
		}
		wrote = true
		return nil
	})
	if err != nil {
		return syncFailed(m, err)
	}
	if wrote {
		s.sseHub.publishGuest("checkout", res.ServerRecord, weddingID)
	}
	return res
}

// mergeSyncPayload applies the fields p carries to g, each on its own time: a
// field takes the value only when opTime is newer than that field's own last
// write, so a newer write to another field cannot carry it away (invariant 1).
func mergeSyncPayload(g *models.GuestRecord, p *SyncPayload, opTime time.Time) (syncMerge, error) {
	var r syncMerge
	if p == nil {
		return r, nil
	}
	if p.Name != nil && r.take(g, models.FieldName, opTime) {
		r.changed = r.changed || g.Name != *p.Name
		g.Name = *p.Name
		g.StampField(models.FieldName, opTime)
	}
	if p.Email != nil && r.take(g, models.FieldEmail, opTime) {
		r.changed = r.changed || g.Email != *p.Email
		g.Email = *p.Email
		g.StampField(models.FieldEmail, opTime)
	}
	if p.Pax != nil && r.take(g, models.FieldPax, opTime) {
		r.changed = r.changed || g.Pax != *p.Pax
		g.Pax = *p.Pax
		g.StampField(models.FieldPax, opTime)
	}
	// An empty RSVP means "not set", as online; validateGuestFields checks the rest.
	if p.RSVP != nil && r.take(g, models.FieldRSVP, opTime) {
		r.changed = r.changed || g.RSVP != *p.RSVP
		g.RSVP = *p.RSVP
		g.StampField(models.FieldRSVP, opTime)
	}
	if p.IsVip != nil && r.take(g, models.FieldIsVip, opTime) {
		r.changed = r.changed || g.IsVip != *p.IsVip
		g.IsVip = *p.IsVip
		g.StampField(models.FieldIsVip, opTime)
	}
	if p.Notes != nil && r.take(g, models.FieldNotes, opTime) {
		r.changed = r.changed || g.Notes != *p.Notes
		g.Notes = *p.Notes
		g.StampField(models.FieldNotes, opTime)
	}
	if p.Dietary != nil && r.take(g, models.FieldDietary, opTime) {
		r.changed = r.changed || !slices.Equal(g.Dietary, *p.Dietary)
		g.Dietary = *p.Dietary
		g.StampField(models.FieldDietary, opTime)
	}
	// Present-but-null clears the value; absent leaves it alone.
	if p.AngbaoAmt.Set && r.take(g, models.FieldAngbaoAmt, opTime) {
		r.changed = r.changed || !reflect.DeepEqual(g.AngbaoAmt, p.AngbaoAmt.Value)
		g.AngbaoAmt = p.AngbaoAmt.Value
		g.StampField(models.FieldAngbaoAmt, opTime)
	}
	if p.GiftItem.Set && r.take(g, models.FieldGiftItem, opTime) {
		r.changed = r.changed || !reflect.DeepEqual(g.GiftItem, p.GiftItem.Value)
		g.GiftItem = p.GiftItem.Value
		g.StampField(models.FieldGiftItem, opTime)
	}
	// Only an explicitly sent tableId touches the seat assignment: absent means
	// "leave it alone", null or empty means unassign. The pair moves together.
	if p.TableID.Set && r.take(g, models.FieldSeat, opTime) {
		if p.TableID.Value == nil || *p.TableID.Value == "" {
			r.changed = r.changed || g.TableID != nil
			g.TableID, g.SeatNum = nil, nil
		} else {
			tid, err := utils.DecodeUUID(*p.TableID.Value)
			if err != nil {
				return r, fmt.Errorf("%w %q", errInvalidTableID, *p.TableID.Value)
			}
			r.changed = r.changed || !reflect.DeepEqual(g.TableID, &tid) || !reflect.DeepEqual(g.SeatNum, p.SeatNum)
			g.TableID, g.SeatNum = &tid, p.SeatNum
		}
		g.StampField(models.FieldSeat, opTime)
	}
	return r, nil
}

// mergeSyncCheckIn applies a queued check-in to g: the arrival, the RSVP and
// the angbao and gift it carries, each on its own time. Notes append instead of
// replacing (invariant 3), and a note already present is a replay and is not
// added again (invariant 5). The caller validates the merged row.
func mergeSyncCheckIn(g *models.GuestRecord, p *SyncPayload, opTime time.Time) (syncMerge, error) {
	var r syncMerge
	if r.take(g, models.FieldCheckedIn, opTime) {
		r.changed = r.changed || g.CheckedInAt == nil || !g.CheckedInAt.Equal(opTime)
		g.CheckedInAt = &opTime
		g.StampField(models.FieldCheckedIn, opTime)
	}
	if r.take(g, models.FieldRSVP, opTime) {
		r.changed = r.changed || g.RSVP != "confirmed"
		g.RSVP = "confirmed"
		g.StampField(models.FieldRSVP, opTime)
	}
	if p == nil {
		return r, nil
	}
	if p.AngbaoAmt.Set && r.take(g, models.FieldAngbaoAmt, opTime) {
		r.changed = r.changed || !reflect.DeepEqual(g.AngbaoAmt, p.AngbaoAmt.Value)
		g.AngbaoAmt = p.AngbaoAmt.Value
		g.StampField(models.FieldAngbaoAmt, opTime)
	}
	if p.GiftItem.Set && r.take(g, models.FieldGiftItem, opTime) {
		r.changed = r.changed || !reflect.DeepEqual(g.GiftItem, p.GiftItem.Value)
		g.GiftItem = p.GiftItem.Value
		g.StampField(models.FieldGiftItem, opTime)
	}
	if p.Notes != nil {
		note := strings.TrimSpace(*p.Notes)
		if note != "" && !slices.Contains(strings.Split(g.Notes, "\n"), note) {
			// Offline, nobody can shorten the note and retry: cut it to fit
			// rather than refuse the check-in and lose the arrival and angbao.
			if fitted := fitNote(g.Notes, note); fitted != note {
				log.Printf("sync check-in of guest %s: note cut from %d to %d characters to fit notes limit", g.ID, utf8.RuneCountInString(note), utf8.RuneCountInString(fitted))
				note = fitted
			}
			g.Notes = appendNote(g.Notes, note)
			r.changed = true
		}
		g.StampField(models.FieldNotes, opTime)
	}
	return r, nil
}

// validateSyncPayload runs the online PUT rules over the fields p carries, so an
// offline edit is refused like an online one even when last-write-wins would
// drop it anyway (invariant 7).
func validateSyncPayload(p *SyncPayload) error {
	if p == nil {
		return nil
	}
	scratch := &models.GuestRecord{}
	if err := applySyncPayload(scratch, p, nil); err != nil {
		return err
	}
	return validateGuestFields(scratch)
}

// newSyncGuest builds the row a queued create carries.
func newSyncGuest(weddingID, gid uuid.UUID, p *SyncPayload, opTime time.Time) (*models.GuestRecord, error) {
	g := &models.GuestRecord{
		ID:        gid,
		WeddingID: weddingID,
		Name:      *p.Name,
		Pax:       1,
		AngbaoAmt: p.AngbaoAmt.Value,
		GiftItem:  p.GiftItem.Value,
		CreatedAt: opTime,
		UpdatedAt: opTime,
	}
	if p.Pax != nil && *p.Pax >= 1 {
		g.Pax = *p.Pax
	}
	if p.Email != nil {
		g.Email = *p.Email
	}
	if p.RSVP != nil {
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
	if t := p.TableID; t.Set && t.Value != nil && *t.Value != "" {
		tid, err := utils.DecodeUUID(*t.Value)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid tableId %q", errInvalidTableID, *t.Value)
		}
		g.TableID = &tid
		g.SeatNum = p.SeatNum
	}
	return g, nil
}

func applySyncPayload(g *models.GuestRecord, p *SyncPayload, gid *uuid.UUID) error {
	// Same rules as online PUT: a blank name or out-of-range pax is
	// refused, not silently dropped and reported as applied.
	if p.Name != nil {
		if strings.TrimSpace(*p.Name) == "" {
			return fmt.Errorf("%w: name is required", ErrInvalidGuestField)
		}
		g.Name = *p.Name
	}
	if p.Email != nil {
		g.Email = *p.Email
	}
	if p.Pax != nil {
		if err := validatePax(*p.Pax); err != nil {
			return err
		}
		g.Pax = *p.Pax
	}
	// Empty RSVP means "not set", as online; validateGuestFields checks the rest.
	if p.RSVP != nil {
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

// fitNote cuts note to the room left under MaxGuestNotesLen once it is
// appended to existing with appendNote.
func fitNote(existing, note string) string {
	room := MaxGuestNotesLen - utf8.RuneCountInString(existing)
	if existing != "" {
		room-- // the joining newline
	}
	if room <= 0 {
		return ""
	}
	if r := []rune(note); len(r) > room {
		return string(r[:room])
	}
	return note
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
