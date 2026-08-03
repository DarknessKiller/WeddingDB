package handlers

import (
	"strings"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

type WeddingHandler struct {
	weddingService *services.WeddingService
	adminRepo      *repository.AdminRepo
}

func NewWeddingHandler(weddingService *services.WeddingService, adminRepo *repository.AdminRepo) *WeddingHandler {
	return &WeddingHandler{weddingService: weddingService, adminRepo: adminRepo}
}

type WeddingRequest struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

func (h *WeddingHandler) List(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	role := RoleFromContext(ctx)
	if role == "admin" {
		return h.weddingService.List(ctx)
	}
	// user: only return their own wedding from JWT
	wid := WeddingIDFromContext(ctx)
	if wid == nil {
		return []models.WeddingEvent{}, nil
	}
	w, err := h.weddingService.Get(ctx, *wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	return []models.WeddingEvent{*w}, nil
}

func (h *WeddingHandler) Get(c fuego.ContextNoBody) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, id); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	return h.weddingService.Get(ctx, id)
}

func (h *WeddingHandler) Create(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	d, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid date format, use YYYY-MM-DD"}
	}
	w := &models.WeddingEvent{Name: body.Name, Date: d}
	if err := h.weddingService.Create(ctx, w); err != nil {
		return nil, err
	}
	// Link the creating admin to this wedding
	adminID := AdminIDFromContext(c.Context())
	if adminID != uuid.Nil {
		h.adminRepo.AddUserWedding(ctx, adminID, w.ID)
	}
	c.SetStatus(201)
	return w, nil
}

func (h *WeddingHandler) Update(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, id); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w, err := h.weddingService.Get(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	w.Name = name
	if body.Date != "" {
		d, err := time.Parse("2006-01-02", body.Date)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid date format, use YYYY-MM-DD"}
		}
		w.Date = d
	}
	if err := h.weddingService.Update(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

type KioskSettingsRequest struct {
	Name                *string `json:"name"` // nil = unchanged (dirty-only from frontend)
	VenueName           string  `json:"venueName"`
	VenueAddress        string `json:"venueAddress"`
	KioskDescription    string `json:"kioskDescription"`
	KioskLogoUrl        string `json:"kioskLogoUrl"`
	KioskBackgroundUrl  string `json:"kioskBackgroundUrl"`
	KioskBackgroundBlur int    `json:"kioskBackgroundBlur"`
	KioskBackgroundSize string `json:"kioskBackgroundSize"`
	KioskBackgroundPosX string `json:"kioskBackgroundPosX"`
	KioskBackgroundPosY string `json:"kioskBackgroundPosY"`
	KioskLogoSize       string `json:"kioskLogoSize"`
	KioskLogoPosX       string `json:"kioskLogoPosX"`
	KioskLogoPosY       string `json:"kioskLogoPosY"`
	ShowSeatNumbers     *bool  `json:"showSeatNumbers"`
}

func (h *WeddingHandler) UpdateKioskSettings(c fuego.ContextWithBody[KioskSettingsRequest]) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, id); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w, err := h.weddingService.Get(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			return nil, fuego.BadRequestError{Title: "Name is required"}
		}
		w.Name = name
	}
	w.VenueName = strings.TrimSpace(body.VenueName)
	w.VenueAddress = strings.TrimSpace(body.VenueAddress)
	w.KioskDescription = strings.TrimSpace(body.KioskDescription)
	w.KioskLogoUrl = sanitizeURL(body.KioskLogoUrl)
	w.KioskBackgroundUrl = sanitizeURL(body.KioskBackgroundUrl)
	w.KioskBackgroundBlur = max(0, min(20, body.KioskBackgroundBlur))
	w.KioskBackgroundSize = validateCSSValue(body.KioskBackgroundSize, validBackgroundSizes, "cover")
	w.KioskBackgroundPosX = validateCSSValue(body.KioskBackgroundPosX, validPositionsX, "center")
	w.KioskBackgroundPosY = validateCSSValue(body.KioskBackgroundPosY, validPositionsY, "center")
	w.KioskLogoSize = validateCSSValue(body.KioskLogoSize, validBackgroundSizes, "contain")
	w.KioskLogoPosX = validateCSSValue(body.KioskLogoPosX, validPositionsX, "center")
	w.KioskLogoPosY = validateCSSValue(body.KioskLogoPosY, validPositionsY, "center")
	if body.ShowSeatNumbers != nil {
		w.ShowSeatNumbers = *body.ShowSeatNumbers
	}
	if err := h.weddingService.Update(ctx, w); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update kiosk settings"}
	}
	return w, nil
}

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.weddingService.Delete(ctx, id); err != nil {
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

// sanitizeURL strips any value that isn't a safe URL prefix or a relative path.
// Prevents CSS injection via url() in style attributes.
func sanitizeURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return ""
	}
	allowed := strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./")
	if !allowed {
		return ""
	}
	// Block values containing CSS-breaking characters
	if strings.ContainsAny(s, ")\"';}{") {
		return ""
	}
	return s
}

var (
	validBackgroundSizes = map[string]bool{"cover": true, "contain": true, "auto": true}
	validPositionsX      = map[string]bool{"left": true, "center": true, "right": true}
	validPositionsY      = map[string]bool{"top": true, "center": true, "bottom": true}
)

func validateCSSValue(val string, allowed map[string]bool, fallback string) string {
	if allowed[val] {
		return val
	}
	return fallback
}
