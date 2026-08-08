package handlers

import (
	"errors"
	"strconv"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type GuestHandler struct{ guestService *services.GuestService }

func NewGuestHandler(guestService *services.GuestService) *GuestHandler {
	return &GuestHandler{guestService: guestService}
}

type GuestCreateRequest struct {
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
	if body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 {
		return nil, fuego.BadRequestError{Title: "Pax must be at least 1"}
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
		Phone:     body.Phone,
		Email:     body.Email,
		Pax:       body.Pax,
		RSVP:      body.RSVP,
		IsVip:     body.IsVip,
		Notes:     body.Notes,
		Dietary:   body.Dietary,
	}
	if err := h.guestService.Create(ctx, guest); err != nil {
		return nil, err
	}
	c.SetStatus(201)
	if body.TableID != nil && *body.TableID != "" {
		if tid, err := DecodeID(*body.TableID); err == nil {
			seatNum := 1
			if body.SeatNum != nil {
				seatNum = *body.SeatNum
			}
			if err := h.guestService.AssignSeat(ctx, guest.ID, wid, tid, seatNum); err == nil {
				guest.TableID = &tid
				guest.SeatNum = &seatNum
			}
		}
	}
	return guest, nil
}

func (h *GuestHandler) Update(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
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
	validRSVP := map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}
	if body.RSVP != "" && !validRSVP[body.RSVP] {
		return nil, fuego.BadRequestError{Title: "Invalid RSVP status"}
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
	guest.Phone = body.Phone
	guest.Email = body.Email
	guest.Pax = body.Pax
	guest.RSVP = body.RSVP
	guest.IsVip = body.IsVip
	guest.Notes = body.Notes
	guest.Dietary = body.Dietary
	guest.AngbaoAmt = body.AngbaoAmt
	guest.GiftItem = body.GiftItem
	if body.TableID != nil {
		if *body.TableID == "" {
			// Explicitly clearing seat
			guest.TableID = nil
			guest.SeatNum = nil
		} else if tid, err := DecodeID(*body.TableID); err == nil {
			seatNum := 1
			if body.SeatNum != nil {
				seatNum = *body.SeatNum
			}
			if err := h.guestService.AssignSeat(ctx, guest.ID, wid, tid, seatNum); err != nil {
				return nil, fuego.BadRequestError{Title: err.Error()}
			}
			guest.TableID = &tid
			guest.SeatNum = &seatNum
		}
	}
	// else: tableId not provided — preserve existing seat assignment
	if err := h.guestService.Update(ctx, guest); err != nil {
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
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

type CheckInRequest struct {
	AngbaoAmt *int    `json:"angbaoAmt"`
	GiftItem  *string `json:"giftItem"`
}

func (h *GuestHandler) CheckIn(c fuego.ContextWithBody[CheckInRequest]) (any, error) {
	ctx := c.Context()
	body, _ := c.Body()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if body.AngbaoAmt != nil || body.GiftItem != nil {
		guest, err := h.guestService.Get(ctx, id, wid)
		if err != nil {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		if body.AngbaoAmt != nil {
			guest.AngbaoAmt = body.AngbaoAmt
		}
		if body.GiftItem != nil {
			guest.GiftItem = body.GiftItem
		}
		if err := h.guestService.Update(ctx, guest); err != nil {
			return nil, err
		}
	}
	if err := h.guestService.CheckIn(ctx, id, wid); err != nil {
		if errors.Is(err, services.ErrAlreadyCheckedIn) {
			return nil, fuego.ConflictError{Title: "Guest already checked in by another receptionist"}
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
	if len(body.Guests) > 1000 {
		return nil, fuego.BadRequestError{Title: "Maximum 1000 guests per import"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	var guests []models.GuestRecord
	for _, g := range body.Guests {
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
			Phone:     g.Phone,
			Email:     g.Email,
			Pax:       g.Pax,
			RSVP:      g.RSVP,
			IsVip:     g.IsVip,
			Notes:     g.Notes,
			Dietary:   g.Dietary,
		}
		if g.TableID != nil {
			if tid, err := DecodeID(*g.TableID); err == nil {
				gr.TableID = &tid
			}
		}
		if g.SeatNum != nil {
			gr.SeatNum = g.SeatNum
		}
		guests = append(guests, gr)
	}
	count, err := h.guestService.BulkCreate(ctx, guests)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Bulk import failed"}
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
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	c.SetStatus(204)
	return nil, nil
}
