package handlers

import (
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type WeddingHandler struct{ weddingService *services.WeddingService }

func NewWeddingHandler(weddingService *services.WeddingService) *WeddingHandler {
	return &WeddingHandler{weddingService: weddingService}
}

type WeddingRequest struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

func (h *WeddingHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	role := RoleFromContext(ctx)
	if role == "service_admin" {
		return h.weddingService.List()
	}
	// wedding_admin: only return their own wedding
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
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
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

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	role := RoleFromContext(c.Context())
	if role != "service_admin" {
		return nil, fuego.UnauthorizedError{Title: "service_admin role required"}
	}
	id := DecodeID(c.PathParam("id"))
	return nil, h.weddingService.Delete(id)
}
