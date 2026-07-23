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
	Name string    `json:"n"`
	Date time.Time `json:"d"`
}

func (h *WeddingHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	return h.weddingService.List()
}

func (h *WeddingHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	id := DecodeID(c.PathParam("id"))
	return h.weddingService.Get(id)
}

func (h *WeddingHandler) Create(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w := &models.WeddingEvent{Name: body.Name, Date: body.Date}
	if err := h.weddingService.Create(w); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *WeddingHandler) Update(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	id := DecodeID(c.PathParam("id"))
	w, err := h.weddingService.Get(id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	w.Name = body.Name
	w.Date = body.Date
	if err := h.weddingService.Update(w); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	id := DecodeID(c.PathParam("id"))
	return nil, h.weddingService.Delete(id)
}
