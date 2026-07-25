package handlers

import (
	"strconv"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
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

func (h *GuestHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	limit := 100
	cursor := c.QueryParam("cursor")
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	guests, total, err := h.guestService.List(wid, cursor, limit)
	if err != nil {
		return nil, err
	}
	var nextCursor string
	if len(guests) > limit {
		nextCursor = guests[limit].ID.String()
		guests = guests[:limit]
	}
	return map[string]any{"guests": guests, "total": total, "nextCursor": nextCursor}, nil
