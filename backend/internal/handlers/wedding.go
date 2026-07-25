package handlers

import (
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

func (h *WeddingHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	role := RoleFromContext(ctx)
	if role == "admin" {
		return h.weddingService.List()
	}
	// user: only return their own wedding from JWT
	wid := WeddingIDFromContext(ctx)
	if wid == nil {
		return []models.WeddingEvent{}, nil
	}
	w, err := h.weddingService.Get(*wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	return []models.WeddingEvent{*w}, nil
}

func (h *WeddingHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	id := DecodeID(c.PathParam("id"))
	if err := requireWeddingAccess(c.Context(), id); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	return h.weddingService.Get(id)
}

func (h *WeddingHandler) Create(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
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
	if err := h.weddingService.Create(w); err != nil {
		return nil, err
	}
	// Link the creating admin to this wedding
	adminID := AdminIDFromContext(c.Context())
	if adminID != uuid.Nil {
		h.adminRepo.AddUserWedding(adminID, w.ID)
	}
	return w, nil
}

func (h *WeddingHandler) Update(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	id := DecodeID(c.PathParam("id"))
	if err := requireWeddingAccess(c.Context(), id); err != nil {
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
	w, err := h.weddingService.Get(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	w.Name = body.Name
	w.Date = d
	if err := h.weddingService.Update(w); err != nil {
		return nil, err
	}
	return w, nil
}

type KioskSettingsRequest struct {
	KioskTitle         string `json:"kioskTitle"`
	KioskDescription   string `json:"kioskDescription"`
	KioskLogoUrl       string `json:"kioskLogoUrl"`
	KioskBackgroundUrl  string `json:"kioskBackgroundUrl"`
	KioskBackgroundBlur  int    `json:"kioskBackgroundBlur"`
	KioskBackgroundSize  string `json:"kioskBackgroundSize"`
	KioskBackgroundPosX  string `json:"kioskBackgroundPosX"`
	KioskBackgroundPosY  string `json:"kioskBackgroundPosY"`
	KioskLogoSize   string `json:"kioskLogoSize"`
	KioskLogoPosX   string `json:"kioskLogoPosX"`
	KioskLogoPosY   string `json:"kioskLogoPosY"`
}

func (h *WeddingHandler) UpdateKioskSettings(c fuego.ContextWithBody[KioskSettingsRequest]) (any, error) {
	id := DecodeID(c.PathParam("id"))
	if err := requireWeddingAccess(c.Context(), id); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w, err := h.weddingService.Get(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	w.KioskTitle = body.KioskTitle
	w.KioskDescription = body.KioskDescription
	w.KioskLogoUrl = body.KioskLogoUrl
	w.KioskBackgroundUrl = body.KioskBackgroundUrl
	w.KioskBackgroundBlur = body.KioskBackgroundBlur
	w.KioskBackgroundSize = body.KioskBackgroundSize
	w.KioskBackgroundPosX = body.KioskBackgroundPosX
	w.KioskBackgroundPosY = body.KioskBackgroundPosY
	w.KioskLogoSize = body.KioskLogoSize
	w.KioskLogoPosX = body.KioskLogoPosX
	w.KioskLogoPosY = body.KioskLogoPosY
	if err := h.weddingService.Update(w); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update kiosk settings"}
	}
	return w, nil
}

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	if err := requireAdmin(c.Context()); err != nil {
		return nil, fuego.UnauthorizedError{Title: err.Error()}
	}
	id := DecodeID(c.PathParam("id"))
	return nil, h.weddingService.Delete(id)
}
