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
		return ruleError{fmt.Errorf("party size must be at most %d", MaxGuestPartySize)}
	}
	if err := ValidateGuestText(g); err != nil {
		return ruleError{err}
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
		return ruleError{guestCapError(total, g.Pax)}
	}
	repo := repository.NewGuestRepo(tx)
	if syncWrite {
		return repo.SyncCreate(ctx, g)
	}
	return repo.Create(ctx, g)
}

func (s *GuestService) updateGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord) error {
	if g.Pax < 1 {
		g.Pax = 1
	}
	if g.Pax > MaxGuestPartySize {
		return ruleError{fmt.Errorf("party size must be at most %d", MaxGuestPartySize)}
	}
	if err := ValidateGuestText(g); err != nil {
		return ruleError{err}
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
		return ruleError{guestCapError(total, g.Pax-current.Pax)}
	}
	repo := repository.NewGuestRepo(tx)
	// Online edits never write checked_in_at; report the stored value.
	g.CheckedInAt = current.CheckedInAt
	if g.UpdatedAt.IsZero() {
		return repo.Update(ctx, g)
	}
	// The caller edited a row loaded at g.UpdatedAt: refuse the write if the
	// row moved on, so a stale form cannot revert another writer.
	return repo.UpdateIfUnchanged(ctx, g, g.UpdatedAt)
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

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Fetch guest before deletion so we can broadcast the event. A missing
	// guest is gorm.ErrRecordNotFound: nothing to delete or broadcast.
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if err != nil {
		return err
	}
	if err := s.guestRepo.Delete(ctx, id, weddingID); err != nil {
		return err
	}
	s.publishEvent("delete", guest, weddingID)
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
			return ruleError{errors.New("table not found")}
		}
	}
	table, err := repository.NewTableRepo(tx).FindByID(ctx, tableID, weddingID)
	if err != nil {
		return ruleError{errors.New("table not found")}
	}
	if pax < 1 {
		return ruleError{errors.New("guest pax must be at least 1")}
	}
	if seatNum < 1 || seatNum > table.Capacity-pax+1 {
		return ruleError{errors.New("seat range exceeds table capacity")}
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
			return ruleError{fmt.Errorf("%w: \"%s\" (seats %d-%d)", ErrSeatOverlap, e.Name, *e.SeatNum, eEnd)}
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
		// but a cross-wedding table id must still be rejected. Lock the row like
		// checkSeatTx does: table delete takes the same lock, so a plain
		// assignment cannot slip in while the table is being dropped.
		// The row lock is Postgres-only; sqlite (tests) has one writer.
		if tx.Name() == "postgres" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
				Where("id = ? AND wedding_id = ?", *g.TableID, g.WeddingID).First(&models.BanquetTable{}).Error; err != nil {
				return ruleError{errors.New("table not found")}
			}
		}
		if _, err := repository.NewTableRepo(tx).FindByID(ctx, *g.TableID, g.WeddingID); err != nil {
			return ruleError{errors.New("table not found")}
		}
		return nil
	}
	return checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, g.ID, g.Pax, *g.SeatNum)
}

func (s *GuestService) AssignSeat(ctx context.Context, guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	var guest *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGuestRepo := repository.NewGuestRepo(tx)
		// Wedding row lock keeps the lock order identical to create/update, so
		// a concurrent seat move and import cannot deadlock each other.
		if _, err := s.guestCountLocked(tx, weddingID); err != nil {
			return err
		}
		// Lock the table before reading the guest: the guest's pax decides the
		// seat range, so it must be read under the lock, not before it.
		if tx.Name() == "postgres" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
				Where("id = ? AND wedding_id = ?", tableID, weddingID).First(&models.BanquetTable{}).Error; err != nil {
				return ruleError{errors.New("table not found")}
			}
		}
		var err error
		guest, err = txGuestRepo.FindByIDForUpdate(ctx, guestID, weddingID)
		if err != nil {
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
		return s.updateGuestTx(ctx, tx, g)
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
	notes = strings.TrimSpace(notes)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		// Lock the row before reading its notes: the append below must see the
		// notes a concurrent edit committed, or it would overwrite them.
		guest, err := repo.FindByIDForUpdate(ctx, id, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		if err != nil {
			return err
		}
		if guest.CheckedInAt != nil {
			return ErrAlreadyCheckedIn
		}
		fields := map[string]any{"checked_in_at": now, "rsvp": "confirmed"}
		if notes != "" {
			fields["notes"] = appendNote(guest.Notes, notes)
		}
		return repo.UpdateFields(ctx, id, weddingID, fields)
	})
	if err != nil {
		return err
	}
	s.publishFresh(ctx, "checkin", id, weddingID)
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
	GuestID string `json:"guestId"`
	Status  string `json:"status"` // applied | skipped | failed
	Reason  string `json:"reason,omitempty"`
	// Retryable tells the client whether to keep a failed mutation queued:
	// false when the mutation breaks a rule and would fail on every retry.
	Retryable    bool                `json:"retryable"`
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

// ruleError marks a mutation that breaks a guest rule (validation, seat,
// capacity), so retrying it cannot succeed. Its message is the wrapped one.
type ruleError struct{ error }

func (e ruleError) Unwrap() error { return e.error }

// IsRuleError reports whether err is a broken guest rule (validation, seat,
// capacity), which the handler answers with 400 instead of 500.
func IsRuleError(err error) bool {
	_, ok := errors.AsType[ruleError](err)
	return ok
}

// syncFailed answers a mutation that did not apply. Only errors a later attempt
// can clear (database, network) are retryable.
func syncFailed(m SyncMutation, err error) SyncResult {
	_, rule := errors.AsType[ruleError](err)
	retryable := !rule && !errors.Is(err, gorm.ErrRecordNotFound)
	return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: err.Error(), Retryable: retryable}
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
		return syncFailed(m, err)
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

	var (
		res        SyncResult
		eventKind  string
		eventGuest *models.GuestRecord
	)
	// One transaction for the read, the LWW comparison and the write, with the
	// guest row locked (syncLoadForUpdate): two devices' mutations cannot both
	// pass the comparison against the same pre-write snapshot, and the write
	// never carries a row older than the lock.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		svc := &GuestService{guestRepo: repository.NewGuestRepo(tx), tableRepo: repository.NewTableRepo(tx), sseHub: s.sseHub, db: tx}
		existing, found, loadErr := svc.syncLoadForUpdate(ctx, tx, gid, weddingID)
		if loadErr != nil {
			return loadErr
		}

		applied := func(g *models.GuestRecord, kind string) error {
			res = SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}
			eventKind, eventGuest = kind, g
			return nil
		}
		skipped := func(reason string, g *models.GuestRecord) error {
			res = SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: reason, ServerRecord: g}
			return nil
		}

		switch m.Op {
		case SyncOpCreate, SyncOpUpdate:
			if !found {
				if m.Op == SyncOpUpdate {
					return skipped("not found", nil)
				}
				return svc.syncCreate(ctx, tx, weddingID, gid, m, opTime, applied, skipped)
			}
			return svc.syncUpdate(ctx, tx, weddingID, existing, m, opTime, applied, skipped)

		case SyncOpDelete:
			if !found {
				return applied(nil, "")
			}
			if opTime.Before(repository.NewestWrite(existing)) {
				// Report "failed", not "skipped": the client keeps failed items
				// queued instead of dequeuing and losing the delete intent.
				// Not retryable: the server copy only gets newer, so a retry loses again.
				res = SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "older than server", ServerRecord: existing, Retryable: true} // spec invariant 3: an older delete stays queued
				return nil
			}
			if err := svc.guestRepo.Delete(ctx, gid, weddingID); err != nil {
				return err
			}
			return applied(existing, "delete")

		case SyncOpCheckIn:
			if !found {
				return skipped("not found", nil)
			}
			fields := make(map[string]any, 5)
			if !opTime.Before(repository.FieldVersion(existing, "checked_in_at")) {
				existing.CheckedInAt = &opTime
				fields["checked_in_at"] = opTime
			}
			if !opTime.Before(repository.FieldVersion(existing, "rsvp")) {
				existing.RSVP = "confirmed"
				fields["rsvp"] = "confirmed"
			}
			if m.Payload != nil {
				if m.Payload.has("angbaoAmt") && m.Payload.AngbaoAmt != nil &&
					!opTime.Before(repository.FieldVersion(existing, "angbao_amt")) {
					existing.AngbaoAmt = m.Payload.AngbaoAmt
					fields["angbao_amt"] = *m.Payload.AngbaoAmt
				}
				if m.Payload.has("giftItem") && m.Payload.GiftItem != nil &&
					!opTime.Before(repository.FieldVersion(existing, "gift_item")) {
					existing.GiftItem = m.Payload.GiftItem
					fields["gift_item"] = *m.Payload.GiftItem
				}
				// A notes value in a checkin payload is appended to the guest's
				// notes, never replacing them, so a queued note never wipes
				// existing notes.
				note := strings.TrimSpace(m.Payload.Notes)
				if note != "" && !opTime.Before(repository.FieldVersion(existing, "notes")) {
					if err := checkLen("Check-in notes", note, MaxCheckInNoteLen); err != nil {
						return ruleError{err}
					}
					existing.Notes = appendNote(existing.Notes, note)
					fields["notes"] = existing.Notes
				}
			}
			if len(fields) == 0 {
				return skipped("older than server", existing)
			}
			if err := ValidateGuestText(existing); err != nil {
				return ruleError{err}
			}
			rowTime := newerTime(repository.NewestWrite(existing), opTime)
			existing.UpdatedAt = rowTime
			if err := svc.guestRepo.SyncUpdateFields(ctx, gid, weddingID, fields, opTime, rowTime); err != nil {
				return err
			}
			return applied(existing, "checkin")

		case SyncOpCheckOut:
			if !found {
				return skipped("not found", nil)
			}
			if opTime.Before(repository.FieldVersion(existing, "checked_in_at")) {
				return skipped("older than server", existing)
			}
			existing.CheckedInAt = nil
			rowTime := newerTime(repository.NewestWrite(existing), opTime)
			existing.UpdatedAt = rowTime
			if err := svc.guestRepo.SyncUpdateFields(ctx, gid, weddingID, map[string]any{"checked_in_at": nil}, opTime, rowTime); err != nil {
				return err
			}
			return applied(existing, "checkout")

		default:
			return skipped("unknown op", nil)
		}
	})
	if err != nil {
		return syncFailed(m, err)
	}
	if eventKind != "" {
		s.publishEvent(eventKind, eventGuest, weddingID)
	}
	return res
}

// syncLoadForUpdate loads a guest under a row lock so the LWW comparison and
// the write that follows it see the same row. The lock is Postgres-only;
// sqlite (tests) has one writer.
func (s *GuestService) syncLoadForUpdate(ctx context.Context, tx *gorm.DB, gid, weddingID uuid.UUID) (*models.GuestRecord, bool, error) {
	q := tx.WithContext(ctx)
	if tx.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var g models.GuestRecord
	err := q.Where("id = ? AND wedding_id = ?", gid, weddingID).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &g, true, nil
}

func newerTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// applySyncFields merges the payload fields whose client time is not older
// than the stored field version into g, and records the columns to write.
// Fields the payload does not carry keep their stored value and version.
func applySyncFields(g *models.GuestRecord, p *SyncPayload, opTime time.Time, fields map[string]any) error {
	newer := func(column string) bool { return !opTime.Before(repository.FieldVersion(g, column)) }
	if p.has("name") && p.Name != "" && newer("name") {
		g.Name = p.Name
		g.NamePinyin = models.GenerateNamePinyin(p.Name)
		fields["name"] = g.Name
		fields["name_pinyin"] = g.NamePinyin
	}
	if p.has("email") && newer("email") {
		g.Email = p.Email
		fields["email"] = p.Email
	}
	if p.has("pax") && p.Pax >= 1 && newer("pax") {
		g.Pax = p.Pax
		fields["pax"] = p.Pax
	}
	if p.has("rsvp") && p.RSVP != "" && newer("rsvp") {
		g.RSVP = p.RSVP
		fields["rsvp"] = p.RSVP
	}
	if p.has("notes") && newer("notes") {
		g.Notes = p.Notes
		fields["notes"] = p.Notes
	}
	if p.has("dietary") && newer("dietary") {
		g.Dietary = p.Dietary
		fields["dietary"] = p.Dietary
	}
	if p.has("isVip") && newer("is_vip") {
		g.IsVip = p.IsVip
		fields["is_vip"] = p.IsVip
	}
	if p.has("angbaoAmt") && newer("angbao_amt") {
		g.AngbaoAmt = p.AngbaoAmt
		fields["angbao_amt"] = g.AngbaoAmt
	}
	if p.has("giftItem") && newer("gift_item") {
		g.GiftItem = p.GiftItem
		fields["gift_item"] = g.GiftItem
	}
	if !p.has("tableId") {
		return nil
	}
	if p.TableID == nil || *p.TableID == "" {
		// JSON null and empty string both mean "unassign": clear the whole
		// seat assignment. SeatNum must never survive with a nil TableID.
		if newer("table_id") || newer("seat_num") {
			g.TableID = nil
			g.SeatNum = nil
			fields["table_id"] = nil
			fields["seat_num"] = nil
		}
		return nil
	}
	tid, err := parseSyncID(*p.TableID)
	if err != nil {
		return ruleError{fmt.Errorf("invalid tableId %q", *p.TableID)}
	}
	if newer("table_id") {
		g.TableID = &tid
		fields["table_id"] = tid
	}
	if p.has("seatNum") && newer("seat_num") {
		g.SeatNum = p.SeatNum
		fields["seat_num"] = p.SeatNum
	}
	return nil
}

// validateSyncUpdate runs the same rules as the online edit path against the
// merged row, so an offline mutation cannot bypass them.
func (s *GuestService) validateSyncUpdate(ctx context.Context, tx *gorm.DB, g *models.GuestRecord, previousPax int, fields map[string]any) error {
	_, tableWritten := fields["table_id"]
	_, seatWritten := fields["seat_num"]
	_, paxWritten := fields["pax"]
	if tableWritten || seatWritten || paxWritten {
		if err := checkSeatNotTaken(ctx, tx, g); err != nil {
			return err
		}
	}
	if g.Pax < 1 {
		g.Pax = 1
	}
	if g.Pax > MaxGuestPartySize {
		return ruleError{fmt.Errorf("party size must be at most %d", MaxGuestPartySize)}
	}
	if err := ValidateGuestText(g); err != nil {
		return ruleError{err}
	}
	total, err := s.guestCountLocked(tx, g.WeddingID)
	if err != nil {
		return err
	}
	if g.Pax > previousPax && total-previousPax+g.Pax > MaxGuestsPerWedding {
		return ruleError{guestCapError(total, g.Pax-previousPax)}
	}
	return nil
}

type syncApplyFn func(*models.GuestRecord, string) error
type syncSkipFn func(string, *models.GuestRecord) error

func (s *GuestService) syncUpdate(ctx context.Context, tx *gorm.DB, weddingID uuid.UUID, existing *models.GuestRecord, m SyncMutation, opTime time.Time, applied syncApplyFn, skipped syncSkipFn) error {
	previousPax := existing.Pax
	fields := make(map[string]any, 11)
	if m.Payload != nil {
		if err := applySyncFields(existing, m.Payload, opTime, fields); err != nil {
			return err
		}
	}
	if len(fields) == 0 {
		return skipped("older than server", existing)
	}
	if err := s.validateSyncUpdate(ctx, tx, existing, previousPax, fields); err != nil {
		return err
	}
	rowTime := newerTime(repository.NewestWrite(existing), opTime)
	existing.UpdatedAt = rowTime
	if err := s.guestRepo.SyncUpdateFields(ctx, existing.ID, weddingID, fields, opTime, rowTime); err != nil {
		return err
	}
	return applied(existing, "update")
}

func (s *GuestService) syncCreate(ctx context.Context, tx *gorm.DB, weddingID, gid uuid.UUID, m SyncMutation, opTime time.Time, applied syncApplyFn, skipped syncSkipFn) error {
	if m.Payload == nil || m.Payload.Name == "" {
		return skipped("name required", nil)
	}
	if m.Payload.Pax < 1 {
		m.Payload.Pax = 1
	}
	if err := s.validateSyncTableSeat(ctx, weddingID, m.Payload); err != nil {
		return err
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
	if err := s.createGuestTx(ctx, tx, g, true); err != nil {
		return err
	}
	return applied(g, "create")
}

// validateSyncTableSeat mirrors the table/seat rules of AssignSeat for offline
// sync creates: the table must exist in this wedding and the seat must fit
// its capacity. A nil/empty tableId (unassign) needs no validation.
func (s *GuestService) validateSyncTableSeat(ctx context.Context, weddingID uuid.UUID, p *SyncPayload) error {
	if p.TableID == nil || *p.TableID == "" {
		return nil
	}
	tid, err := parseSyncID(*p.TableID)
	if err != nil {
		return ruleError{fmt.Errorf("invalid tableId %q", *p.TableID)}
	}
	table, err := s.tableRepo.FindByID(ctx, tid, weddingID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ruleError{fmt.Errorf("table %s not found", *p.TableID)}
	}
	if err != nil {
		return fmt.Errorf("find table %s: %w", *p.TableID, err)
	}
	if p.SeatNum != nil && table.Capacity > 0 && (*p.SeatNum < 1 || *p.SeatNum > table.Capacity) {
		return ruleError{fmt.Errorf("seat %d exceeds capacity %d of table %s", *p.SeatNum, table.Capacity, table.Name)}
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
