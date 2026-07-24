package handlers

import (
	"time"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type publicGuest struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Phone       string     `json:"phone"`
	TableID     *string    `json:"tableId"`
	SeatNum     *int       `json:"seatNum"`
	Pax         int        `json:"pax"`
	IsVip       bool       `json:"isVip"`
	CheckedInAt *time.Time `json:"checkedInAt"`
}

type PublicGuestHandler struct {
	guestService *services.GuestService
}

func NewPublicGuestHandler(guestService *services.GuestService) *PublicGuestHandler {
	return &PublicGuestHandler{guestService: guestService}
}

func (h *PublicGuestHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	guests, _, err := h.guestService.List(wid, 0, 1000)
	if err != nil {
		return nil, err
	}
	out := make([]publicGuest, 0, len(guests))
	for _, g := range guests {
		var tid *string
		if g.TableID != nil {
			s := g.TableID.String()
			tid = &s
		}
		out = append(out, publicGuest{
			ID:          g.ID.String(),
			Name:        g.Name,
			Phone:       g.Phone,
			TableID:     tid,
			SeatNum:     g.SeatNum,
			Pax:         g.Pax,
			IsVip:       g.IsVip,
			CheckedInAt: g.CheckedInAt,
		})
	}
	return out, nil
}

func (h *PublicGuestHandler) Search(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(wid, query)
	if err != nil {
		return nil, err
	}
	out := make([]publicGuest, 0, len(guests))
	for _, g := range guests {
		var tid *string
		if g.TableID != nil {
			s := g.TableID.String()
			tid = &s
		}
		out = append(out, publicGuest{
			ID:          g.ID.String(),
			Name:        g.Name,
			Phone:       g.Phone,
			TableID:     tid,
			SeatNum:     g.SeatNum,
			Pax:         g.Pax,
			IsVip:       g.IsVip,
			CheckedInAt: g.CheckedInAt,
		})
	}
	return out, nil
}
