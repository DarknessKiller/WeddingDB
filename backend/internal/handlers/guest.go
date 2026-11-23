package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

type GuestHandler struct{ guestService *services.GuestService }

func NewGuestHandler(guestService *services.GuestService) *GuestHandler {
	return &GuestHandler{guestService: guestService}
}

type GuestCreateRequest struct {
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
}

func (h *GuestHandler) List(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	limit := 100
	cursor := c.QueryParam("cursor")
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	guests, total, err := h.guestService.List(ctx, wid, cursor, limit)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidCursor) {
			return nil, fuego.BadRequestError{Title: "Invalid cursor"}
		}
		return nil, err
	}
	var nextCursor string
	if len(guests) > limit {
		nextCursor = guests[limit-1].ID.String()
		guests = guests[:limit]
	}
	return map[string]any{"guests": guests, "total": total, "nextCursor": nextCursor}, nil
}

func (h *GuestHandler) Get(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	guest, err := h.guestService.Get(ctx, id, wid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	if err != nil {
		slog.ErrorContext(ctx, "load guest failed", "guest", id, "wedding", wid, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to load guest"}
	}
	return guest, nil
}

func (h *GuestHandler) Create(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 {
		return nil, fuego.BadRequestError{Title: "Pax must be at least 1"}
	}
	if body.Pax > services.MaxGuestPartySize {
		return nil, fuego.BadRequestError{Title: "Pax must be at most " + strconv.Itoa(services.MaxGuestPartySize)}
	}
	validRSVP := map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}
	if body.RSVP != "" && !validRSVP[body.RSVP] {
		return nil, fuego.BadRequestError{Title: "Invalid RSVP status"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	guest := &models.GuestRecord{
		WeddingID: wid,
		Name:      body.Name,
		Email:     body.Email,
		Pax:       body.Pax,
		RSVP:      body.RSVP,
		IsVip:     body.IsVip,
		Notes:     body.Notes,
		Dietary:   body.Dietary,
	}
	if err := services.ValidateGuestText(guest); err != nil {
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	// Decode the seat before creating so guest and seat land in one transaction.
	if body.TableID != nil && *body.TableID != "" {
		tid, err := DecodeID(*body.TableID)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid table ID"}
		}
		seatNum := 1
		if body.SeatNum != nil {
			seatNum = *body.SeatNum
		}
		guest.TableID = &tid
		guest.SeatNum = &seatNum
	}
	if err := h.guestService.Create(ctx, guest); err != nil {
		var lenErr services.FieldLenError
		if errors.As(err, &lenErr) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, services.ErrGuestCapExceeded) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, services.ErrSeatOverlap) || errors.Is(err, services.ErrTableNotFound) ||
			errors.Is(err, services.ErrInvalidGuestPax) || errors.Is(err, services.ErrSeatRange) ||
			errors.Is(err, services.ErrGuestPatchInvalid) {
			return nil, seatAssignmentError(err)
		}
		return nil, err
	}
	c.SetStatus(201)
	return guest, nil
}

// Update applies a sparse edit: absent fields keep their stored value, explicit
// null clears a nullable field, and the merged record is returned.
func (h *GuestHandler) Update(c fuego.ContextWithBody[*services.GuestPatch]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil || body == nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	guest, err := h.guestService.UpdatePatch(ctx, id, wid, body)
	if err != nil {
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		if errors.Is(err, services.ErrGuestPatchInvalid) ||
			errors.Is(err, services.ErrGuestCapExceeded) ||
			errors.Is(err, services.ErrSeatOverlap) ||
			errors.Is(err, services.ErrTableNotFound) ||
			errors.Is(err, services.ErrInvalidGuestPax) ||
			errors.Is(err, services.ErrSeatRange) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		var lenErr services.FieldLenError
		if errors.As(err, &lenErr) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.guestService.Delete(ctx, id, wid); err != nil {
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

// CheckInRequest is the optional body of the check-in endpoint.
type CheckInRequest struct {
	Notes string `json:"notes"`
}

// CheckIn checks a guest in, promoting their RSVP to confirmed. Non-empty notes
// are appended to the guest's notes. Gift/angpao keying is a separate
// flow — it goes through the guest-update endpoint (PUT /guests/{id}), never
// this one.
func (h *GuestHandler) CheckIn(c fuego.ContextWithBody[CheckInRequest]) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	// Body is optional (older clients send none): ignore decode errors.
	body, err := c.Body()
	if err != nil {
		slog.Warn("check-in body decode failed, continuing without notes", "guest", id, "err", err)
	}
	if len([]rune(body.Notes)) > services.MaxCheckInNoteLen {
		return nil, fuego.BadRequestError{Title: "Check-in notes must be at most " + strconv.Itoa(services.MaxCheckInNoteLen) + " characters"}
	}
	if err := h.guestService.CheckIn(ctx, id, wid, body.Notes); err != nil {
		if errors.Is(err, services.ErrAlreadyCheckedIn) {
			return nil, fuego.ConflictError{Title: "Guest already checked in by another receptionist"}
		}
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		var lenErr services.FieldLenError
		if errors.As(err, &lenErr) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		return nil, err
	}
	guest, err := h.guestService.Get(ctx, id, wid)
	if err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) CheckOut(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.guestService.CheckOut(ctx, id, wid); err != nil {
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

func (h *GuestHandler) Search(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(ctx, wid, query)
	if err != nil {
		return nil, err
	}
	return guests, nil
}

func (h *GuestHandler) Occupancy(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	occ, err := h.guestService.Occupancy(ctx, wid)
	if err != nil {
		return nil, err
	}
	if occ == nil {
		occ = []repository.TableOccupancy{}
	}
	return occ, nil
}

type BulkImportRequest struct {
	Guests []GuestCreateRequest `json:"guests"`
}

func (h *GuestHandler) BulkImport(c fuego.ContextWithBody[BulkImportRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if len(body.Guests) == 0 {
		return map[string]any{"imported": 0}, nil
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	var guests []models.GuestRecord
	for i, g := range body.Guests {
		if g.Name == "" {
			continue
		}
		if g.Pax < 1 {
			g.Pax = 1
		}
		validRSVP := map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}
		if g.RSVP != "" && !validRSVP[g.RSVP] {
			g.RSVP = "no_response"
		}
		gr := models.GuestRecord{
			WeddingID: wid,
			Name:      g.Name,
			Email:     g.Email,
			Pax:       g.Pax,
			RSVP:      g.RSVP,
			IsVip:     g.IsVip,
			Notes:     g.Notes,
			Dietary:   g.Dietary,
		}
		if g.SeatNum != nil && g.TableID == nil {
			return nil, fuego.BadRequestError{Title: "Invalid guests[" + strconv.Itoa(i) + "].seatNum: tableId is required"}
		}
		if g.TableID != nil {
			tid, err := DecodeID(*g.TableID)
			if err != nil {
				return nil, fuego.BadRequestError{Title: "Invalid guests[" + strconv.Itoa(i) + "].tableId"}
			}
			gr.TableID = &tid
		}
		if g.SeatNum != nil {
			gr.SeatNum = g.SeatNum
		}
		guests = append(guests, gr)
	}
	count, err := h.guestService.BulkCreate(ctx, guests)
	if err != nil {
		var lenErr services.FieldLenError
		if errors.As(err, &lenErr) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, services.ErrGuestCapExceeded) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, services.ErrSeatOverlap) || errors.Is(err, services.ErrSeatRange) ||
			errors.Is(err, services.ErrTableNotFound) || errors.Is(err, services.ErrGuestPatchInvalid) ||
			errors.Is(err, services.ErrInvalidGuestPax) {
			return nil, seatAssignmentError(err)
		}
		slog.ErrorContext(ctx, "bulk import failed", "err", err)
		return nil, fuego.InternalServerError{Title: "Bulk import failed"}
	}
	return map[string]any{"imported": count}, nil
}

type AssignSeatRequest struct {
	TableID string `json:"tableId"`
	SeatNum int    `json:"seatNum"`
}

func (h *GuestHandler) AssignSeat(c fuego.ContextWithBody[AssignSeatRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	guestID, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	tableID, err := DecodeID(body.TableID)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid table ID"}
	}
	if err := h.guestService.AssignSeat(ctx, guestID, wid, tableID, body.SeatNum); err != nil {
		return nil, seatAssignmentError(err)
	}
	c.SetStatus(204)
	return nil, nil
}

func seatAssignmentError(err error) error {
	if errors.Is(err, services.ErrSeatOverlap) || errors.Is(err, services.ErrTableNotFound) || errors.Is(err, services.ErrInvalidGuestPax) || errors.Is(err, services.ErrSeatRange) || errors.Is(err, services.ErrGuestCapExceeded) || errors.Is(err, services.ErrGuestPatchInvalid) {
		return fuego.BadRequestError{Title: err.Error()}
	}
	return err
}

func (h *GuestHandler) UnassignSeat(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	guestID, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.guestService.UnassignSeat(ctx, guestID, wid); err != nil {
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

type SyncRequest struct {
	// Raw items so one undecodable mutation fails alone (Offline sync invariant 8).
	Mutations []json.RawMessage `json:"mutations"`
}

// maxSyncMutations matches the client chunk and keeps one batch well under the
// 1 MiB body limit (Offline sync).
const maxSyncMutations = 100

func (h *GuestHandler) Sync(c fuego.ContextWithBody[SyncRequest]) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if len(body.Mutations) > maxSyncMutations {
		return nil, fuego.BadRequestError{Title: "Too many mutations (max 100)"}
	}
	results := make([]services.SyncResult, len(body.Mutations))
	valid := make([]services.SyncMutation, 0, len(body.Mutations))
	at := make([]int, 0, len(body.Mutations))
	for i, raw := range body.Mutations {
		var m services.SyncMutation
		if err := json.Unmarshal(raw, &m); err != nil {
			results[i] = services.SyncResult{Status: "failed", Reason: "malformed mutation", Retryable: new(false)}
			continue
		}
		valid = append(valid, m)
		at = append(at, i)
	}
	applied, err := h.guestService.Sync(ctx, wid, valid)
	if err != nil {
		return nil, err
	}
	for j, r := range applied {
		results[at[j]] = r
	}
	return map[string]any{"results": results}, nil
}
