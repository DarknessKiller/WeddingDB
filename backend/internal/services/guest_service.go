package services

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgconn"
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

func guestCapError(current, delta int) error {
	return fmt.Errorf("%w: this wedding has %d guests; this change would make %d, over the %d-guest limit",
		ErrGuestCapExceeded, current, current+delta, MaxGuestsPerWedding)
}

// weddingLockError names a deleted wedding instead of reporting the guest that
// could not exist under it.
func weddingLockError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrWeddingNotFound
	}
	return err
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
	if err := repository.LockWedding(tx, weddingID); err != nil {
		return 0, weddingLockError(err)
	}
	var total int
	if err := tx.Model(&models.GuestRecord{}).
		Where("wedding_id = ?", weddingID).
		Select("COALESCE(SUM(pax), 0)").Row().Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// stampCreateFieldTimes gives a new row its own time for every LWW group, so a
// later partial edit cannot make untouched columns look newer than a write that
// predates it. A caller that already stamped (sync create) keeps its times.
func stampCreateFieldTimes(g *models.GuestRecord) {
	if len(g.FieldTimes) != 0 {
		return
	}
	now := time.Now()
	g.FieldTimes = make(models.FieldTimes, len(guestGroups()))
	for _, key := range guestGroups() {
		g.FieldTimes[key] = now
	}
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
	stampCreateFieldTimes(g)
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrGuestNotFound
		}
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

func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, cursor string, limit int, withTotal bool) ([]models.GuestRecord, int64, error) {
	return s.guestRepo.ListByWedding(ctx, weddingID, cursor, limit, withTotal)
}

func (s *GuestService) Get(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	return s.guestRepo.FindByID(ctx, id, weddingID)
}

// Create inserts g and reports whether it made a new row. A client-chosen ID
// that already names a guest in this wedding is a retried create: g becomes the
// stored guest and nothing is written or published. An ID that belongs to
// another wedding is ErrGuestIDTaken.
func (s *GuestService) Create(ctx context.Context, g *models.GuestRecord) (bool, error) {
	var existing *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if g.ID != uuid.Nil {
			// Read under the wedding lock, so a concurrent retry of the same
			// create sees the row the first one inserted.
			if err := repository.LockWedding(tx, g.WeddingID); err != nil {
				return weddingLockError(err)
			}
			found, err := repository.NewGuestRepo(tx).FindByID(ctx, g.ID, g.WeddingID)
			if err == nil {
				existing = found
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		return s.createGuestTx(ctx, tx, g)
	})
	if err != nil {
		if g.ID != uuid.Nil && isGuestIDViolation(err) {
			return false, ErrGuestIDTaken
		}
		return false, err
	}
	if existing != nil {
		*g = *existing
		return false, nil
	}
	s.sseHub.publishGuest("create", g, g.WeddingID)
	return true, nil
}

// isGuestIDViolation reports a guest insert that hit the primary key. Guest
// rows have no other unique constraint, so this is an ID another wedding owns.
func isGuestIDViolation(err error) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23505" && pgErr.TableName == "guest_records"
	}
	// sqlite, used in tests, exposes the column but not a constraint name.
	return strings.Contains(err.Error(), "UNIQUE constraint failed: guest_records.id")
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
	var guest *models.GuestRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.LockWedding(tx, weddingID); err != nil {
			return weddingLockError(err)
		}
		repo := repository.NewGuestRepo(tx)
		locked, err := repo.FindByIDForUpdate(ctx, id, weddingID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		if err != nil {
			return err
		}
		if err := repo.Delete(ctx, id, weddingID, time.Now()); err != nil {
			return err
		}
		guest = locked
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	s.sseHub.publishGuest("delete", guest, weddingID)
	return nil
}

func (s *GuestService) Search(ctx context.Context, weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	return s.guestRepo.SearchByWedding(ctx, weddingID, capSearchQuery(query))
}

// maxSearchQueryRunes bounds search input: the repo runs a pinyin lookup per
// character plus a leading-wildcard ILIKE, so an unbounded query is pure load.
const maxSearchQueryRunes = 100

// capSearchQuery truncates by runes, not bytes, so a Chinese name is never cut
// mid-character.
func capSearchQuery(query string) string {
	if r := []rune(query); len(r) > maxSearchQueryRunes {
		return string(r[:maxSearchQueryRunes])
	}
	return query
}

// checkSeatTx validates a seat assignment inside tx and returns the table row
// lock. Every write path that sets a seat calls this, so all of them serialize
// on the same table row and share the capacity/overlap rules.
func checkSeatTx(ctx context.Context, tx *gorm.DB, weddingID, tableID uuid.UUID, guestID uuid.UUID, pax int, seatNum *int) error {
	seats, err := loadTableSeats(ctx, tx, weddingID, tableID)
	if err != nil {
		return err
	}
	return seats.check(guestID, pax, seatNum)
}

// tableSeats is a table's capacity and seated guests, read under the table
// row lock so it stays current for the rest of the transaction.
type tableSeats struct {
	capacity  int
	occupants []models.GuestRecord
}

func loadTableSeats(ctx context.Context, tx *gorm.DB, weddingID, tableID uuid.UUID) (*tableSeats, error) {
	if err := lockTable(tx, weddingID, tableID); err != nil {
		return nil, err
	}
	table, err := repository.NewTableRepo(tx).FindByID(ctx, tableID, weddingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}
	existing, err := repository.NewGuestRepo(tx).FindByTable(ctx, weddingID, tableID)
	if err != nil {
		return nil, err
	}
	return &tableSeats{capacity: table.Capacity, occupants: existing}, nil
}

// check applies the capacity and overlap rules to one guest's seat range. A nil
// seatNum is a table without a seat: only the headcount must fit, counting
// every guest on the table, seated or not.
func (t *tableSeats) check(guestID uuid.UUID, pax int, seatNum *int) error {
	if pax < 1 {
		return ErrInvalidGuestPax
	}
	total := pax
	for _, e := range t.occupants {
		if e.ID != guestID {
			total += e.Pax
		}
	}
	if total > t.capacity {
		return fmt.Errorf("%w: the table holds %d, its guests would total %d", ErrSeatRange, t.capacity, total)
	}
	if seatNum == nil {
		return nil
	}
	// Bound seatNum before adding pax: a huge value would overflow past the check.
	if *seatNum < 1 || *seatNum > t.capacity || *seatNum+pax-1 > t.capacity {
		return ErrSeatRange
	}
	start, guestEnd := *seatNum, *seatNum+pax-1
	for _, e := range t.occupants {
		if e.ID == guestID || e.SeatNum == nil {
			continue
		}
		eEnd := *e.SeatNum + e.Pax - 1
		if start <= eEnd && *e.SeatNum <= guestEnd {
			return fmt.Errorf("%w: \"%s\" (seats %d-%d)", ErrSeatOverlap, e.Name, *e.SeatNum, eEnd)
		}
	}
	return nil
}

// checkSeatNotTaken enforces the seat rules for a whole guest row. A table
// with or without a seat goes through the same locked check as AssignSeat, so
// headcount and cross-wedding table ids are checked either way.
func checkSeatNotTaken(ctx context.Context, tx *gorm.DB, g *models.GuestRecord) error {
	if g.TableID == nil {
		return nil
	}
	return checkSeatTx(ctx, tx, g.WeddingID, *g.TableID, g.ID, g.Pax, g.SeatNum)
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
// Returns the merged row, and ErrAlreadyCheckedIn if the guest was already
// checked in by another receptionist (FIFO conflict).
func (s *GuestService) CheckIn(ctx context.Context, id, weddingID uuid.UUID, notes string) (*models.GuestRecord, error) {
	notes = strings.TrimSpace(notes)
	if err := checkLen("Check-in notes", notes, MaxCheckInNoteLen); err != nil {
		return nil, err
	}
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.LockWedding(tx, weddingID); err != nil {
			return weddingLockError(err)
		}
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGuestNotFound
		}
		return nil, err
	}
	if applied {
		s.sseHub.publishGuest("checkin", merged, weddingID)
	}
	return merged, nil
}

func (s *GuestService) CheckOut(ctx context.Context, id, weddingID uuid.UUID) error {
	var merged *models.GuestRecord
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.LockWedding(tx, weddingID); err != nil {
			return weddingLockError(err)
		}
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGuestNotFound
		}
		return err
	}
	if applied {
		s.sseHub.publishGuest("checkout", merged, weddingID)
	}
	return nil
}

// bulkResyncThreshold is the import size above which one resync replaces
// per-row events; each client buffers 64 events before it is disconnected.
const bulkResyncThreshold = 50

// BulkCreate imports guests in one transaction. A row whose client-minted ID
// already names a guest in this wedding is skipped, so re-uploading after a lost
// response does not duplicate the list. An ID another wedding owns is
// ErrGuestIDTaken.
func (s *GuestService) BulkCreate(ctx context.Context, guests []models.GuestRecord) (int, error) {
	created := 0
	if len(guests) == 0 {
		return 0, nil
	}
	weddingID := guests[0].WeddingID
	ids := make([]uuid.UUID, len(guests))
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
		// Minted here, not by the database, so the batch's own rows count
		// toward a table's headcount as distinct guests.
		if guests[i].ID == uuid.Nil {
			guests[i].ID = uuid.New()
		}
		ids[i] = guests[i].ID
		stampCreateFieldTimes(&guests[i])
	}
	// Buffer SSE events and publish them only after the transaction commits,
	// so a rolled-back batch never broadcasts phantom rows.
	events := make([]*models.GuestRecord, 0, len(guests))

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txGuestRepo := repository.NewGuestRepo(tx)
		created = 0
		events = events[:0]
		total, err := s.guestCountLocked(tx, weddingID)
		if err != nil {
			return err
		}
		// Read under the wedding lock, so a concurrent retry of the same
		// import sees the rows the first one inserted.
		var found []uuid.UUID
		if err := tx.Model(&models.GuestRecord{}).Where("wedding_id = ? AND id IN ?", weddingID, ids).
			Pluck("id", &found).Error; err != nil {
			return err
		}
		existing := make(map[uuid.UUID]bool, len(found))
		for _, id := range found {
			existing[id] = true
		}
		incoming := 0
		for i := range guests {
			if !existing[guests[i].ID] {
				incoming += guests[i].Pax
			}
		}
		if total+incoming > MaxGuestsPerWedding {
			return guestCapError(total, incoming)
		}
		// Each table's occupancy is read once under its row lock, so an import
		// serializes on the same rows as AssignSeat, then grows with the rows
		// this batch inserts. Rows with or without a seat share the headcount.
		seats := make(map[uuid.UUID]*tableSeats)
		for i := range guests {
			g := &guests[i]
			if existing[g.ID] {
				continue
			}
			if g.TableID != nil {
				tid := *g.TableID
				ts := seats[tid]
				if ts == nil {
					if ts, err = loadTableSeats(ctx, tx, g.WeddingID, tid); err != nil {
						return err
					}
					seats[tid] = ts
				}
				if err := ts.check(g.ID, g.Pax, g.SeatNum); err != nil {
					return err
				}
				ts.occupants = append(ts.occupants, *g)
			}
			if err := txGuestRepo.Create(ctx, g); err != nil {
				return err
			}
			events = append(events, g)
			created++
		}
		return nil
	})
	if err != nil {
		if isGuestIDViolation(err) {
			return 0, ErrGuestIDTaken
		}
		// Transaction rolled back: nothing committed, nothing to broadcast.
		return 0, err
	}
	if len(events) > bulkResyncThreshold && s.sseHub != nil {
		// One resync instead of a burst that would overflow every client's
		// event buffer and force the same reconnect anyway.
		s.sseHub.reconnectWedding(weddingID)
		s.sseHub.broadcastResync(context.Background(), weddingID)
		return created, nil
	}
	for _, g := range events {
		s.sseHub.publishGuest("create", g, weddingID)
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

// syncErrorKnown reports a validation or business error the client can act on
// and may see verbatim. Anything else is an outage and stays generic.
func syncErrorKnown(err error) bool {
	var fieldErr FieldLenError
	if errors.As(err, &fieldErr) {
		return true
	}
	return errors.Is(err, ErrGuestPatchInvalid) || errors.Is(err, ErrGuestCapExceeded) || errors.Is(err, ErrSeatOverlap) || errors.Is(err, ErrTableNotFound) || errors.Is(err, ErrInvalidGuestPax) || errors.Is(err, ErrSeatRange) || errors.Is(err, repository.ErrCapacityBelowSeated) || errors.Is(err, ErrGuestIDTaken)
}

func syncErrorReason(ctx context.Context, err error) string {
	if syncErrorKnown(err) {
		return err.Error()
	}
	slog.ErrorContext(ctx, "sync mutation failed", "error", err)
	return "Internal error"
}

// syncErrorRetryable reports whether re-sending a failed mutation could ever
// succeed. Validation rejects the same input every time, so the client drops
// those instead of retrying them forever. Guest-cap and seat-overlap conflicts
// are retryable: another guest may be deleted or move off the seat meanwhile.
func syncErrorRetryable(err error) bool {
	if !syncErrorKnown(err) {
		return true
	}
	return errors.Is(err, ErrGuestCapExceeded) || errors.Is(err, ErrSeatOverlap)
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

// errSyncCreateNameRequired fails a malformed create inside the transaction,
// after the existence check, so a retried create for a stored guest still merges.
var errSyncCreateNameRequired = errors.New("name is required")

// errSyncCreateDeleted marks a create older than the guest's delete.
var errSyncCreateDeleted = errors.New("guest was deleted")

func (s *GuestService) applySyncMutation(ctx context.Context, weddingID uuid.UUID, m SyncMutation) SyncResult {
	gid, err := utils.DecodeUUID(m.GuestID)
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
		// A client ID is final, so a retried create can arrive after the first
		// attempt inserted. Check for the stored row under the same wedding lock
		// the insert takes; a check outside the transaction can miss it, and the
		// insert then fails on the primary key and looks permanent to the client.
		var (
			g        *models.GuestRecord
			existing bool
		)
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := repository.LockWedding(tx, weddingID); err != nil {
				return weddingLockError(err)
			}
			repo := repository.NewGuestRepo(tx)
			_, err := repo.FindByID(ctx, gid, weddingID)
			if err == nil {
				existing = true
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// A replayed create must not bring back a guest deleted after it.
			// Ties go to the delete, as in guestDeleteWins.
			deletedAt, err := repo.DeletedAt(ctx, gid, weddingID)
			if err == nil && !deletedAt.Before(opTime) {
				return errSyncCreateDeleted
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if m.Payload == nil || strings.TrimSpace(m.Payload.Name) == "" {
				return errSyncCreateNameRequired
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
			g = &models.GuestRecord{
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
				return err
			}
			return s.createGuestTx(ctx, tx, g)
		})
		if errors.Is(err, errSyncCreateNameRequired) {
			return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "name is required", Retryable: new(false)}
		}
		if errors.Is(err, errSyncCreateDeleted) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "deleted"}
		}
		if err != nil {
			// A deleted wedding can never make this create succeed: drop it
			// instead of blocking the offline queue forever.
			if errors.Is(err, ErrWeddingNotFound) {
				return SyncResult{GuestID: m.GuestID, Status: "failed", Reason: "wedding not found", Retryable: new(false)}
			}
			// Another wedding owns the id: the online create answers 409 for this.
			if isGuestIDViolation(err) {
				err = ErrGuestIDTaken
			}
			return failedSync(ctx, m.GuestID, err)
		}
		if existing {
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
		}
		s.sseHub.publishGuest("create", g, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: g}

	case SyncOpUpdate:
		merged, applied, err := s.syncMerge(ctx, weddingID, gid, m.Payload, opTime)
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrGuestNotFound) || errors.Is(err, ErrWeddingNotFound) {
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
			if err := repository.LockWedding(tx, weddingID); err != nil {
				return err
			}
			repo := repository.NewGuestRepo(tx)
			locked, err := repo.FindByIDForUpdate(ctx, gid, weddingID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if !guestDeleteWins(locked, opTime) {
				// The delete's time never moves and the row's only grow, so a
				// retry can never win: skip it and let the client restore the row.
				losing = locked
				return nil
			}
			if err := repo.Delete(ctx, gid, weddingID, opTime); err != nil {
				return err
			}
			deleted = locked
			return nil
		})
		if err != nil {
			// A deleted wedding means the delete intent is already satisfied;
			// retrying it forever would only re-queue the mutation.
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return SyncResult{GuestID: m.GuestID, Status: "applied"}
			}
			return failedSync(ctx, m.GuestID, err)
		}
		if losing != nil {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "edited after the delete", ServerRecord: losing}
		}
		if deleted != nil {
			s.sseHub.publishGuest("delete", deleted, weddingID)
		}
		return SyncResult{GuestID: m.GuestID, Status: "applied"}

	case SyncOpCheckIn:
		var merged *models.GuestRecord
		applied, alreadyIn := false, false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := repository.LockWedding(tx, weddingID); err != nil {
				return weddingLockError(err)
			}
			repo := repository.NewGuestRepo(tx)
			locked, err := repo.FindByIDForUpdate(ctx, gid, weddingID)
			if err != nil {
				return err
			}
			merged = locked
			// Like online CheckIn: a guest already checked in keeps their arrival
			// time and notes. A replay from a device whose clock runs ahead would
			// otherwise look newer and append its note a second time. Angbao and
			// gift still merge by LWW so a later offline entry is not lost.
			alreadyIn = locked.CheckedInAt != nil
			candidate := *locked
			var carried []string
			if !alreadyIn {
				candidate.CheckedInAt = &opTime
				candidate.RSVP = "confirmed"
				carried = []string{groupCheckedInAt, groupRSVP}
			}
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
				if notes := strings.TrimSpace(m.Payload.Notes); notes != "" && !alreadyIn && opTime.After(fieldTime(locked, groupCheckedInAt)) {
					if err := checkLen("Check-in notes", notes, MaxCheckInNoteLen); err != nil {
						return err
					}
					candidate.Notes = appendNote(locked.Notes, notes)
					carried = append(carried, groupNotes)
				}
			}
			if won := mergeGuestWrite(locked, &candidate, opTime, carried); len(won) == 0 {
				return nil
			}
			if err := ValidateGuestText(locked); err != nil {
				return err
			}
			applied = true
			return repo.Update(ctx, locked)
		})
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrWeddingNotFound) {
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: "not found"}
		}
		if err != nil {
			return failedSync(ctx, m.GuestID, err)
		}
		if !applied {
			reason := "older than server"
			if alreadyIn {
				reason = "already checked in"
			}
			return SyncResult{GuestID: m.GuestID, Status: "skipped", Reason: reason, ServerRecord: merged}
		}
		s.sseHub.publishGuest("checkin", merged, weddingID)
		return SyncResult{GuestID: m.GuestID, Status: "applied", ServerRecord: merged}

	case SyncOpCheckOut:
		var merged *models.GuestRecord
		applied := false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := repository.LockWedding(tx, weddingID); err != nil {
				return err
			}
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
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrWeddingNotFound) {
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
