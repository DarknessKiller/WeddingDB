package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	sseHub    *SSEHub
	db        *gorm.DB
}

func NewGuestService(guestRepo *repository.GuestRepo, sseHub *SSEHub, db *gorm.DB) *GuestService {
	return &GuestService{guestRepo: guestRepo, sseHub: sseHub, db: db}
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

// lockWedding takes the wedding row lock every guest write takes first.
// The row lock is Postgres-only; sqlite (tests) has one writer.
func lockWedding(tx *gorm.DB, weddingID uuid.UUID) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").Where("id = ?", weddingID).First(&models.WeddingEvent{}).Error
}

// lockTable takes the table row lock that seat writes serialize on.
// The row lock is Postgres-only; sqlite (tests) has one writer.
func lockTable(tx *gorm.DB, weddingID, tableID uuid.UUID) error {
	if tx.Name() != "postgres" {
		return nil
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").
		Where("id = ? AND wedding_id = ?", tableID, weddingID).First(&models.BanquetTable{}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTableNotFound
	}
	return err
}

// guestCountLocked serializes guest additions for one wedding and returns the
// current count.
func (s *GuestService) guestCountLocked(tx *gorm.DB, weddingID uuid.UUID) (int, error) {
	if err := lockWedding(tx, weddingID); err != nil {
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

func (s *GuestService) createGuestTx(ctx context.Context, tx *gorm.DB, g *models.GuestRecord) error {
	if g.Pax < 1 {
		g.Pax = 1
	}
	if g.Pax > MaxGuestPartySize {
		return fmt.Errorf("%w: party size must be at most %d", ErrGuestPatchInvalid, MaxGuestPartySize)
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
	return repository.NewGuestRepo(tx).Create(ctx, g)
}

// mergeGuestUpdate is the one write path for guest edits: lock the wedding,
// lock the guest row, merge the carried groups, then validate and save the
// merged row. A write that wins no group is not validated and not saved.
func (s *GuestService) mergeGuestUpdate(ctx context.Context, tx *gorm.DB, weddingID, id uuid.UUID, patch *GuestPatch, at time.Time) (*models.GuestRecord, bool, error) {
	total, err := s.guestCountLocked(tx, weddingID)
	if err != nil {
		return nil, false, err
	}
	repo := repository.NewGuestRepo(tx)
	locked, err := repo.FindByIDForUpdate(ctx, id, weddingID)
	if err != nil {
		return nil, false, err
	}
	if patch == nil {
		return locked, false, nil
	}
	existingPax := locked.Pax
	candidate := *locked
	carried, err := guestPatchForWrite(patch).applyTo(&candidate)
	if err != nil {
		return nil, false, err
	}
	won := mergeGuestWrite(locked, &candidate, at, carried)
	if len(won) == 0 {
		return locked, false, nil
	}
	if locked.Pax > existingPax && total-existingPax+locked.Pax > MaxGuestsPerWedding {
		return nil, false, guestCapError(total, locked.Pax-existingPax)
	}
	// A guest seated past the table's current capacity must still be able to
	// edit every other field, so the seat rules run only when the write moves
	// the seat or changes the party size.
	if slices.Contains(won, groupTableID) || slices.Contains(won, groupPax) {
		if err := checkSeatNotTaken(ctx, tx, locked); err != nil {
			return nil, false, err
		}
	}
	if err := ValidateGuestText(locked); err != nil {
		return nil, false, err
	}
	if err := repo.Update(ctx, locked); err != nil {
		return nil, false, err
	}
	return locked, true, nil
}

// guestPatchForWrite keeps patches built in Go carrying every group without
// failing on the zero RSVP the old full-payload callers never sent. A decoded
// JSON patch keeps its presence map, so an explicit "" is still rejected.
func guestPatchForWrite(p *GuestPatch) *GuestPatch {
	if p == nil || p.set != nil || p.RSVP != "" {
		return p
	}
	c := *p
	c.RSVP = "no_response"
	return &c
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

// UpdatePatch merges a sparse online edit into the locked row and returns the
// merged record. Absent fields keep their stored value, explicit null clears.
func (s *GuestService) UpdatePatch(ctx context.Context, id, weddingID uuid.UUID, patch *GuestPatch) (*models.GuestRecord, error) {
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		merged, applied, err = s.mergeGuestUpdate(ctx, tx, weddingID, id, patch, time.Now())
		return err
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGuestNotFound
		}
		return nil, err
	}
	if applied {
		s.sseHub.publishGuest("update", merged, weddingID)
	}
	return merged, nil
}

func (s *GuestService) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	// Fetch guest before deletion so we can broadcast the event.
	guest, err := s.guestRepo.FindByID(ctx, id, weddingID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrGuestNotFound
	}
	if err != nil {
		return err
	}
	if err := s.guestRepo.Delete(ctx, id, weddingID); err != nil {
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
func checkSeatTx(ctx context.Context, tx *gorm.DB, weddingID, tableID uuid.UUID, guestID uuid.UUID, pax, seatNum int) error {
	if err := lockTable(tx, weddingID, tableID); err != nil {
		return err
	}
	table, err := repository.NewTableRepo(tx).FindByID(ctx, tableID, weddingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTableNotFound
		}
		return err
	}
	if pax < 1 {
		return ErrInvalidGuestPax
	}
	if seatNum < 1 || seatNum+pax-1 > table.Capacity {
		return ErrSeatRange
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
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTableNotFound
			}
			return err
		}
		return nil
	}
	return checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, g.ID, g.Pax, *g.SeatNum)
}

func (s *GuestService) AssignSeat(ctx context.Context, guestID, weddingID, tableID uuid.UUID, seatNum int) error {
	table := tableID.String()
	patch := &GuestPatch{TableID: &table, SeatNum: &seatNum, set: map[string]bool{"tableId": true}}
	var merged *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		// Wedding lock, guest lock, then table lock: the same order as every
		// other write path, so a concurrent seat move and import cannot deadlock.
		merged, _, err = s.mergeGuestUpdate(ctx, tx, weddingID, guestID, patch, time.Now())
		return err
	})
	if err != nil {
		return err
	}
	s.sseHub.publishGuest("seat_assign", merged, weddingID)
	return nil
}

// UnassignSeat clears the table and seat as one column group.
func (s *GuestService) UnassignSeat(ctx context.Context, id, weddingID uuid.UUID) error {
	_, err := s.UpdatePatch(ctx, id, weddingID, &GuestPatch{set: map[string]bool{"tableId": true}})
	return err
}

// CheckIn marks a guest as checked in and promotes their RSVP to confirmed.
// Non-empty notes are appended to the locked guest's existing notes.
// Returns ErrAlreadyCheckedIn if the guest was already checked in by another
// receptionist (FIFO conflict).
func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID, notes string) error {
	notes = strings.TrimSpace(notes)
	if err := checkLen("Check-in notes", notes, MaxCheckInNoteLen); err != nil {
		return err
	}
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		locked, err := repo.FindByIDForUpdate(ctx, id, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		if err != nil {
			return err
		}
		if locked.CheckedInAt != nil {
			return ErrAlreadyCheckedIn
		}
		now := time.Now()
		candidate := *locked
		candidate.CheckedInAt = &now
		candidate.RSVP = "confirmed"
		carried := []string{groupCheckedInAt, groupRSVP}
		if notes != "" {
			candidate.Notes = appendNote(locked.Notes, notes)
			carried = append(carried, groupNotes)
		}
		if err := ValidateGuestText(&candidate); err != nil {
			return err
		}
		merged = locked
		if won := mergeGuestWrite(locked, &candidate, now, carried); len(won) == 0 {
			return nil
		}
		applied = true
		return repo.Update(ctx, locked)
	})
	if err != nil {
		return err
	}
	if applied {
		s.sseHub.publishGuest("checkin", merged, weddingID)
	}
	return nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository.NewGuestRepo(tx)
		locked, err := repo.FindByIDForUpdate(ctx, id, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		if err != nil {
			return err
		}
		now := time.Now()
		candidate := *locked
		candidate.CheckedInAt = nil
		merged = locked
		if won := mergeGuestWrite(locked, &candidate, now, []string{groupCheckedInAt}); len(won) == 0 {
			return nil
		}
		applied = true
		return repo.Update(ctx, locked)
	})
	if err != nil {
		return err
	}
	if applied {
		s.sseHub.publishGuest("checkout", merged, weddingID)
	}
	return nil
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
			return 0, fmt.Errorf("%w: party size must be at most %d", ErrGuestPatchInvalid, MaxGuestPartySize)
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
						if errors.Is(err, gorm.ErrRecordNotFound) {
							return fmt.Errorf("%w: %s", ErrTableNotFound, tid.String())
						}
						return err
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
		s.sseHub.publishGuest(e.eventType, e.guest, e.guest.WeddingID)
	}
	return created, nil
}

func (s *GuestService) Occupancy(ctx context.Context, weddingID uuid.UUID) ([]repository.TableOccupancy, error) {
	return s.guestRepo.TableOccupancy(ctx, weddingID)
}

// publishGuest broadcasts a guest mutation to all connected SSE clients via
// Redis. It is the one publish path for guest and table services. Events go
// through one ordered queue, so a later write is never delivered before an
// earlier one (a check-in followed by its undo). A nil hub publishes nothing.
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

	select {
	case h.publish <- publishRequest{weddingID: weddingID, event: event}:
	case <-h.ctx.Done():
	default:
		// Queue is full: never block the guest write after the DB commit. Drop
		// the event and make this wedding's clients reconnect so they resync
		// instead of keeping a snapshot that missed it. The Redis marker reaches
		// clients on other replicas too; it runs off the request path so a slow
		// Redis cannot block the write.
		slog.Error("SSE: publish queue full; dropping event and forcing client resync",
			"type", eventType, "wedding", weddingID)
		h.reconnectWedding(weddingID)
		go h.broadcastResync(context.Background(), weddingID)
	}
}

// broadcastResync asks every replica, through Redis, to close its clients for
// the wedding so they reconnect and fetch a fresh snapshot.
func (h *SSEHub) broadcastResync(ctx context.Context, weddingID uuid.UUID) {
	if _, loaded := h.resyncInFlight.LoadOrStore(weddingID, struct{}{}); loaded {
		return
	}
	defer h.resyncInFlight.Delete(weddingID)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := h.Publish(ctx, weddingID, GuestEvent{
		Type:      sseResyncType,
		WeddingID: weddingID.String(),
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		slog.ErrorContext(ctx, "SSE: broadcast resync failed", "wedding", weddingID, "err", err)
	}
}

// reconnectWedding closes each local client's Done, forcing a reconnect and
// fresh snapshot.
func (h *SSEHub) reconnectWedding(weddingID uuid.UUID) {
	h.mu.RLock()
	clients := make([]*SSEClient, 0, len(h.clients[weddingID]))
	for _, client := range h.clients[weddingID] {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	for _, client := range clients {
		client.doneOnce.Do(func() { close(client.Done) })
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
	Retryable    *bool               `json:"retryable,omitempty"`
	ServerRecord *models.GuestRecord `json:"serverRecord,omitempty"`
}

// Sync applies a batch of mutations with per-column newer-time-wins.
// Mutations are processed in order received, FIFO.
func (s *GuestService) Sync(ctx context.Context, weddingID uuid.UUID, mutations []SyncMutation) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(mutations))
	for _, m := range mutations {
		res := s.applySyncMutation(ctx, weddingID, m)
		results = append(results, res)
	}
	return results, nil
}

func syncErrorReason(ctx context.Context, err error) string {
	if !syncErrorRetryable(err) {
		return err.Error()
	}
	slog.ErrorContext(ctx, "sync mutation failed", "error", err)
	return "Internal error"
}

// syncErrorRetryable reports whether re-sending a failed mutation could ever
// succeed. Validation and business rules reject the same input every time, so
// the client drops those instead of retrying them forever.
func syncErrorRetryable(err error) bool {
	var fieldErr FieldLenError
	if errors.As(err, &fieldErr) {
		return false
	}
	if errors.Is(err, ErrGuestPatchInvalid) || errors.Is(err, ErrGuestCapExceeded) || errors.Is(err, ErrSeatOverlap) || errors.Is(err, ErrTableNotFound) || errors.Is(err, ErrInvalidGuestPax) || errors.Is(err, ErrSeatRange) || errors.Is(err, repository.ErrCapacityBelowSeated) {
		return false
	}
	return true
}

// failedSync builds a failed result, marking the mutation non-retryable when
// the reason is a rule the input will always break (Offline sync invariant 7).
func failedSync(ctx context.Context, guestID string, err error) SyncResult {
	res := SyncResult{GuestID: guestID, Status: "failed", Reason: syncErrorReason(ctx, err)}
	if !syncErrorRetryable(err) {
		res.Retryable = new(false)
	}
	return res
}

func (s *GuestService) applySyncMutation(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	gid, err := parseSyncID(m.GuestID)
	if err != nil {
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "invalid guestId"}
	}
	if m.ClientUpdatedAt.IsZero() {
		return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "clientUpdatedAt required", Retryable: new(false)}
	}

	// Skew clamp: never trust a client clock in the future, or a skewed device
	// would win every LWW comparison from then on.
	opTime := m.ClientUpdatedAt
	if opTime.After(time.Now()) {
		opTime = time.Now()
	}

	switch m.Op {
	case SyncOpCreate:
		if _, err := s.guestRepo.FindByID(ctx, gid, weddingID); err == nil {
			// The guest already exists: treat the create as an update.
			merged, applied, err := s.syncMerge(ctx, weddingID, gid, m.Payload, opTime)
			if err != nil {
				return failedSync(ctx, m.GuestID, err)
			}
			if !applied {
				return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: merged}
			}
			s.sseHub.publishGuest("update", merged, weddingID)
			return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: merged}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return failedSync(ctx, m.GuestID, err)
		}
		if m.Payload == nil || m.Payload.Name == "" {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "name is required", Retryable: new(false)}
		}
		patch := m.Payload
		if patch.set == nil {
			// Go-built full payloads predate presence tracking: keep the old
			// defaults instead of failing on a zero pax/rsvp.
			c := *patch
			if c.Pax < 1 {
				c.Pax = 1
			}
			if c.RSVP == "" {
				c.RSVP = "no_response"
			}
			patch = &c
		}
		g := &models.GuestRecord{
			ID:         gid,
			WeddingID:  weddingID,
			Pax:        1,
			RSVP:       "no_response",
			CreatedAt:  opTime,
			UpdatedAt:  opTime,
			FieldTimes: make(models.FieldTimes, len(guestGroups())),
		}
		for _, key := range guestGroups() {
			g.FieldTimes[key] = opTime
		}
		if _, err := patch.applyTo(g); err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.createGuestTx(ctx, tx, g)
		}); err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		s.sseHub.publishGuest("create", g, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}

	case SyncOpUpdate:
		merged, applied, err := s.syncMerge(ctx, weddingID, gid, m.Payload, opTime)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if !applied {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: merged}
		}
		s.sseHub.publishGuest("update", merged, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: merged}

	case SyncOpDelete:
		var losing, deleted *models.GuestRecord
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			repo := repository.NewGuestRepo(tx)
			locked, err := repo.FindByIDForUpdate(ctx, gid, weddingID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if !guestDeleteWins(locked, opTime) {
				// Report "failed", not "skipped": the client keeps failed items
				// queued instead of dequeuing and losing the delete intent.
				losing = locked
				return nil
			}
			if err := repo.Delete(ctx, gid, weddingID); err != nil {
				return err
			}
			deleted = locked
			return nil
		})
		if err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if losing != nil {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "older than server", ServerRecord: losing}
		}
		if deleted != nil {
			s.sseHub.publishGuest("delete", deleted, weddingID)
		}
		return SyncResult{GuestID: m.GuestID, Status: "applied"}

	case SyncOpCheckIn:
		var merged *models.GuestRecord
		applied := false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			repo := repository.NewGuestRepo(tx)
			locked, err := repo.FindByIDForUpdate(ctx, gid, weddingID)
			if err != nil {
				return err
			}
			candidate := *locked
			candidate.CheckedInAt = &opTime
			candidate.RSVP = "confirmed"
			carried := []string{groupCheckedInAt, groupRSVP}
			if m.Payload != nil {
				if m.Payload.AngbaoAmt != nil {
					candidate.AngbaoAmt = m.Payload.AngbaoAmt
					carried = append(carried, groupAngbaoAmt)
				}
				if m.Payload.GiftItem != nil {
					candidate.GiftItem = m.Payload.GiftItem
					carried = append(carried, groupGiftItem)
				}
				// A notes value in a checkin payload is appended to the guest's
				// notes, never replacing them, so a queued note never wipes
				// existing notes. A replay carries the same time as the stored
				// check-in, so it must not append the note again.
				if notes := strings.TrimSpace(m.Payload.Notes); notes != "" && opTime.After(fieldTime(locked, groupCheckedInAt)) {
					if err := checkLen("Check-in notes", notes, MaxCheckInNoteLen); err != nil {
						return err
					}
					candidate.Notes = appendNote(locked.Notes, notes)
					carried = append(carried, groupNotes)
				}
			}
			merged = locked
			if won := mergeGuestWrite(locked, &candidate, opTime, carried); len(won) == 0 {
				return nil
			}
			if err := ValidateGuestText(locked); err != nil {
				return err
			}
			applied = true
			return repo.Update(ctx, locked)
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if !applied {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: merged}
		}
		s.sseHub.publishGuest("checkin", merged, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: merged}

	case SyncOpCheckOut:
		var merged *models.GuestRecord
		applied := false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			repo := repository.NewGuestRepo(tx)
			locked, err := repo.FindByIDForUpdate(ctx, gid, weddingID)
			if err != nil {
				return err
			}
			candidate := *locked
			candidate.CheckedInAt = nil
			merged = locked
			if won := mergeGuestWrite(locked, &candidate, opTime, []string{groupCheckedInAt}); len(won) == 0 {
				return nil
			}
			applied = true
			return repo.Update(ctx, locked)
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if !applied {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "older than server", ServerRecord: merged}
		}
		s.sseHub.publishGuest("checkout", merged, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: merged}

	default:
		return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "unknown op"}
	}
}

// syncMerge runs the shared locked merge for one sync payload.
func (s *GuestService) syncMerge(ctx context.Context, weddingID, gid uuid.UUID, payload *SyncPayload, at time.Time) (*models.GuestRecord, bool, error) {
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		merged, applied, err = s.mergeGuestUpdate(ctx, tx, weddingID, gid, payload, at)
		return err
	})
	return merged, applied, err
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
		ID:        utils.EncodeUUID(g.ID),
		Name:      g.Name,
		Email:     g.Email,
		Pax:       g.Pax,
		RSVP:      g.RSVP,
		IsVip:     g.IsVip,
		Notes:     g.Notes,
		Dietary:   []string(g.Dietary),
		Pinyin:    g.NamePinyin,
		UpdatedAt: g.UpdatedAt.Format(time.RFC3339Nano),
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
