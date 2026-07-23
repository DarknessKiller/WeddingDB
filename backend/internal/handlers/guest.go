package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type GuestHandler struct{ guestService *services.GuestService }

func NewGuestHandler(guestService *services.GuestService) *GuestHandler {
	return &GuestHandler{guestService: guestService}
}

type GuestCreateRequest struct {
	Name    string   `json:"n"`
	Phone   string   `json:"p"`
	Email   string   `json:"e"`
	Pax     int      `json:"x"`
	RSVP    string   `json:"r"`
	IsVip   bool     `json:"v"`
	Notes   string   `json:"nt"`
	Dietary []string `json:"d"`
}

func (h *GuestHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	guests, total, err := h.guestService.List(wid, 0, 100)
	if err != nil {
		return nil, err
	}
	return map[string]any{"guests": guests, "total": total}, nil
}

func (h *GuestHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	return guest, nil
}

func (h *GuestHandler) Create(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
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
	if err := h.guestService.Create(guest); err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Update(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
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
	if err := h.guestService.Update(guest); err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.guestService.Delete(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) CheckIn(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.guestService.CheckIn(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) CheckOut(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.guestService.CheckOut(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) Search(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(wid, query)
	if err != nil {
		return nil, err
	}
	return guests, nil
}

func (h *GuestHandler) Occupancy(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	occ, err := h.guestService.Occupancy(wid)
	if err != nil {
		return nil, err
	}
	return occ, nil
}

// AssignSeatRequest is the request body for seat assignment.
type AssignSeatRequest struct {
	TableID string `json:"tableId"`
	SeatNum int    `json:"seatNum"`
}

func (h *GuestHandler) AssignSeat(c fuego.ContextWithBody[AssignSeatRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
	guestID := DecodeID(c.PathParam("id"))
	tableID := DecodeID(body.TableID)
	if err := h.guestService.AssignSeat(guestID, wid, tableID, body.SeatNum); err != nil {
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	return nil, nil
}
