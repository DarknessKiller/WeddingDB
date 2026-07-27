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
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	guests, _, err := h.guestService.List(wid, "", 1000)
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

type PublicKioskHandler struct {
	weddingService *services.WeddingService
}

func NewPublicKioskHandler(weddingService *services.WeddingService) *PublicKioskHandler {
	return &PublicKioskHandler{weddingService: weddingService}
}

func (h *PublicKioskHandler) GetKioskSettings(c fuego.ContextWithBody[any]) (any, error) {
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
	}, nil
}

func (h *PublicGuestHandler) Search(c fuego.ContextWithBody[any]) (any, error) {
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
