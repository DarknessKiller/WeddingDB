package handlers

import (
	"errors"
	"log/slog"
	"strconv"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

type publicGuest struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	TableID *string `json:"tableId"`
	SeatNum *int    `json:"seatNum"`
	Pax     int     `json:"pax"`
	IsVip   bool    `json:"isVip"`
	Pinyin  string  `json:"pinyin"`
}

type PublicGuestHandler struct {
	guestService *services.GuestService
}

func NewPublicGuestHandler(guestService *services.GuestService) *PublicGuestHandler {
	return &PublicGuestHandler{guestService: guestService}
}

func (h *PublicGuestHandler) List(c fuego.ContextNoBody) (any, error) {
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
	if limit > 200 {
		limit = 200
	}
	guests, _, err := h.guestService.List(ctx, wid, cursor, limit, false)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidCursor) {
			return nil, fuego.BadRequestError{Title: "Invalid cursor"}
		}
		slog.ErrorContext(ctx, "public guest list failed", "wedding", wid, "err", err)
		return nil, err
	}
	var nextCursor string
	if len(guests) > limit {
		nextCursor = utils.EncodeUUID(guests[limit-1].ID)
		guests = guests[:limit]
	}
	return map[string]any{"guests": toPublicGuests(guests), "nextCursor": nextCursor}, nil
}

// toPublicGuests is the one model-to-response mapping for public guest lists.
func toPublicGuests(guests []models.GuestRecord) []publicGuest {
	out := make([]publicGuest, 0, len(guests))
	for _, g := range guests {
		var tid *string
		if g.TableID != nil {
			s := utils.EncodeUUID(*g.TableID)
			tid = &s
		}
		out = append(out, publicGuest{
			ID:      utils.EncodeUUID(g.ID),
			Name:    g.Name,
			TableID: tid,
			SeatNum: g.SeatNum,
			Pax:     g.Pax,
			IsVip:   g.IsVip,
			Pinyin:  g.NamePinyin,
		})
	}
	return out
}

type PublicKioskHandler struct {
	weddingService *services.WeddingService
}

func NewPublicKioskHandler(weddingService *services.WeddingService) *PublicKioskHandler {
	return &PublicKioskHandler{weddingService: weddingService}
}

func (h *PublicKioskHandler) GetKioskSettings(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	w, err := h.weddingService.Get(ctx, wid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	if err != nil {
		slog.ErrorContext(ctx, "load wedding failed", "wedding", wid, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to load wedding"}
	}
	return map[string]any{
		"venueName":             w.VenueName,
		"venueAddress":          w.VenueAddress,
		"kioskDescription":      w.KioskDescription,
		"kioskLogoUrl":          w.KioskLogoUrl,
		"kioskBackgroundUrl":    w.KioskBackgroundUrl,
		"kioskBackgroundBlur":   w.KioskBackgroundBlur,
		"kioskBackgroundSize":   w.KioskBackgroundSize,
		"kioskBackgroundPosX":   w.KioskBackgroundPosX,
		"kioskBackgroundPosY":   w.KioskBackgroundPosY,
		"kioskLogoSize":         w.KioskLogoSize,
		"kioskLogoPosX":         w.KioskLogoPosX,
		"kioskLogoPosY":         w.KioskLogoPosY,
		"showSeatNumbers":       w.ShowSeatNumbers,
		"kioskIdleResetSeconds": w.KioskIdleResetSeconds,
		"name":                  w.Name,
		"date":                  w.Date,
	}, nil
}

func (h *PublicGuestHandler) Search(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	query := c.QueryParam("q")
	if r := []rune(query); len(r) > 100 {
		query = string(r[:100])
	}
	guests, err := h.guestService.Search(ctx, wid, query)
	if err != nil {
		return nil, err
	}
	if len(guests) > 50 {
		guests = guests[:50]
	}
	return toPublicGuests(guests), nil
}
