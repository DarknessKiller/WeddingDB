package handlers

import (
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
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
	// BaseUpdatedAt is the updatedAt the editor loaded. When set, an update
	// is refused with 409 if the guest changed since.
	BaseUpdatedAt *time.Time `json:"baseUpdatedAt"`
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
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	return guest, nil
}

func (h *GuestHandler) Create(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 || body.Pax > services.MaxGuestPartySize {
		return nil, fuego.BadRequestError{Title: "Pax must be between 1 and " + strconv.Itoa(services.MaxGuestPartySize)}
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
		AngbaoAmt: body.AngbaoAmt,
		GiftItem:  body.GiftItem,
	}
	if body.TableID != nil && *body.TableID != "" {
		tid, err := DecodeID(*body.TableID)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid table ID"}
		}
		// No seat number means a table-only assignment, as import and sync allow.
		guest.TableID = &tid
		guest.SeatNum = body.SeatNum
	}
	// One transaction creates the guest and checks the seat, so a refused seat
	// leaves no guest behind and no create/delete events go out.
	if err := h.guestService.Create(ctx, guest); err != nil {
		if isGuestRuleError(err) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		return nil, err
	}
	c.SetStatus(201)
	return guest, nil
}

// guestRuleErrors are guest write failures the caller can fix. They answer 400
// with their message; anything else goes to the internal error handler.
var guestRuleErrors = []error{
	services.ErrGuestCapExceeded, services.ErrInvalidPax, services.ErrInvalidGuestField,
	services.ErrSeatOverlap, services.ErrSeatRange, services.ErrTableFull, services.ErrTableNotFound,
}

func isGuestRuleError(err error) bool {
	return slices.ContainsFunc(guestRuleErrors, func(target error) bool { return errors.Is(err, target) })
}

func (h *GuestHandler) Update(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 || body.Pax > services.MaxGuestPartySize {
		return nil, fuego.BadRequestError{Title: "Pax must be between 1 and " + strconv.Itoa(services.MaxGuestPartySize)}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	guest, err := h.guestService.Get(ctx, id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	guest.Name = body.Name
	guest.Email = body.Email
	guest.Pax = body.Pax
	guest.RSVP = body.RSVP
	guest.IsVip = body.IsVip
	guest.Notes = body.Notes
	guest.Dietary = body.Dietary
	guest.AngbaoAmt = body.AngbaoAmt
	guest.GiftItem = body.GiftItem
	// tableId not provided — preserve the existing seat assignment.
	var tid *uuid.UUID
	if body.TableID != nil {
		if *body.TableID == "" {
			// Explicitly clearing seat
			guest.TableID = nil
			guest.SeatNum = nil
		} else {
			id, err := DecodeID(*body.TableID)
			if err != nil {
				return nil, fuego.BadRequestError{Title: "Invalid table ID"}
			}
			tid = &id
		}
	}
	if err := h.guestService.UpdateWithSeat(ctx, guest, tid, body.SeatNum, body.BaseUpdatedAt); err != nil {
		if isGuestRuleError(err) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		if errors.Is(err, services.ErrStaleGuest) {
			return nil, fuego.ConflictError{Title: "Someone else changed this guest. Reload to see their changes."}
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
	if err := h.guestService.CheckIn(ctx, id, wid, body.Notes); err != nil {
		if errors.Is(err, services.ErrAlreadyCheckedIn) {
			return nil, fuego.ConflictError{Title: "Guest already checked in by another receptionist"}
		}
		if errors.Is(err, services.ErrGuestNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		if errors.Is(err, services.ErrInvalidGuestField) {
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
	Guests []BulkImportGuest `json:"guests"`
}

// BulkImportGuest is one CSV line. Row is its spreadsheet row number, so an
// error names the line to fix even after blank and nameless rows are skipped.
type BulkImportGuest struct {
	GuestCreateRequest
	Row int `json:"row"`
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
	var rows []int
	for _, g := range body.Guests {
		if strings.TrimSpace(g.Name) == "" {
			continue
		}
		if g.Pax < 1 {
			g.Pax = 1
		}
		if g.RSVP != "" && !services.ValidRSVP[g.RSVP] {
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
		if g.TableID != nil && *g.TableID != "" {
			tid, err := DecodeID(*g.TableID)
			if err != nil {
				return nil, fuego.BadRequestError{Title: "Invalid table ID"}
			}
			gr.TableID = &tid
			// A seat only means something at a table; drop it otherwise.
			gr.SeatNum = g.SeatNum
		}
		guests = append(guests, gr)
		rows = append(rows, g.Row)
	}
	count, err := h.guestService.BulkCreate(ctx, guests, rows)
	if err != nil {
		if isGuestRuleError(err) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		return nil, err
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
		if isGuestRuleError(err) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
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
	Mutations []services.SyncMutation `json:"mutations"`
}

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
	if len(body.Mutations) > 500 {
		return nil, fuego.BadRequestError{Title: "Too many mutations (max 500)"}
	}
	results, err := h.guestService.Sync(ctx, wid, body.Mutations)
	if err != nil {
		return nil, err
	}
	return map[string]any{"results": results}, nil
}
