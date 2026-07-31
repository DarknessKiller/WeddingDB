package handlers

import (
	"strconv"
	"time"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
)

type publicGuest struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Phone       string     `json:"phone"`
	Rsvp        string     `json:"rsvp"`
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

func (h *PublicGuestHandler) List(c fuego.ContextNoBody) (any, error) {
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
	guests, _, err := h.guestService.List(wid, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	var nextCursor string
	if len(guests) > limit {
		nextCursor = utils.EncodeUUID(guests[limit-1].ID)
		guests = guests[:limit]
	}
	out := make([]publicGuest, 0, len(guests))
	for _, g := range guests {
		var tid *string
		if g.TableID != nil {
			s := utils.EncodeUUID(*g.TableID)
			tid = &s
		}
		out = append(out, publicGuest{
			ID:          utils.EncodeUUID(g.ID),
			Name:        g.Name,
			Phone:       g.Phone,
			Rsvp:        g.RSVP,
			TableID:     tid,
			SeatNum:     g.SeatNum,
			Pax:         g.Pax,
			IsVip:       g.IsVip,
			CheckedInAt: g.CheckedInAt,
		})
	}
	return map[string]any{"guests": out, "nextCursor": nextCursor}, nil
}

type PublicKioskHandler struct {
	weddingService *services.WeddingService
}

func NewPublicKioskHandler(weddingService *services.WeddingService) *PublicKioskHandler {
	return &PublicKioskHandler{weddingService: weddingService}
}

func (h *PublicKioskHandler) GetKioskSettings(c fuego.ContextNoBody) (any, error) {
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	w, err := h.weddingService.Get(wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	return map[string]any{
		"kioskTitle":          w.KioskTitle,
		"kioskDescription":    w.KioskDescription,
		"kioskLogoUrl":        w.KioskLogoUrl,
		"kioskBackgroundUrl":  w.KioskBackgroundUrl,
		"kioskBackgroundBlur": w.KioskBackgroundBlur,
		"kioskBackgroundSize": w.KioskBackgroundSize,
		"kioskBackgroundPosX": w.KioskBackgroundPosX,
		"kioskBackgroundPosY": w.KioskBackgroundPosY,
		"kioskLogoSize":       w.KioskLogoSize,
		"kioskLogoPosX":       w.KioskLogoPosX,
		"kioskLogoPosY":       w.KioskLogoPosY,
		"showSeatNumbers":     w.ShowSeatNumbers,
		"name":                w.Name,
		"date":                w.Date,
	}, nil
}

func (h *PublicGuestHandler) Search(c fuego.ContextNoBody) (any, error) {
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(wid, query)
	if err != nil {
		return nil, err
	}
	out := make([]publicGuest, 0, len(guests))
	for _, g := range guests {
		var tid *string
		if g.TableID != nil {
			s := utils.EncodeUUID(*g.TableID)
			tid = &s
		}
		out = append(out, publicGuest{
			ID:          utils.EncodeUUID(g.ID),
			Name:        g.Name,
			Phone:       g.Phone,
			Rsvp:        g.RSVP,
			TableID:     tid,
			SeatNum:     g.SeatNum,
			Pax:         g.Pax,
			IsVip:       g.IsVip,
			CheckedInAt: g.CheckedInAt,
		})
	}
	return out, nil
}
