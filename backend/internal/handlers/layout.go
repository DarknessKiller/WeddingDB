package handlers

import (
	"cmp"
	"errors"
	"log"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

type LayoutHandler struct {
	layoutService  *services.LayoutService
	tableService   *services.TableService
	weddingService *services.WeddingService
}

func NewLayoutHandler(ls *services.LayoutService, ts *services.TableService, ws *services.WeddingService) *LayoutHandler {
	return &LayoutHandler{layoutService: ls, tableService: ts, weddingService: ws}
}

type TablePos struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Degree float64 `json:"degree"`
	// Capacity and IsVip are written only when sent: the editor sends them
	// for tables it changed, so one opened before a table-form edit does
	// not write its stale copy back.
	Capacity *int  `json:"capacity,omitempty"`
	IsVip    *bool `json:"isVip,omitempty"`
}

type ElementInput struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Degree      float64 `json:"degree"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	TextColor   string  `json:"textColor"`
	StrokeColor string  `json:"strokeColor"`
	Opacity     float64 `json:"opacity"`
	ZIndex      int     `json:"zIndex"`
}

type LayoutRequest struct {
	HallWidth  int            `json:"hallWidth"`
	HallHeight int            `json:"hallHeight"`
	Tables     []TablePos     `json:"tables"`
	Elements   []ElementInput `json:"elements"`
}

type LayoutResponse struct {
	HallWidth  int                   `json:"hallWidth"`
	HallHeight int                   `json:"hallHeight"`
	Tables     []models.BanquetTable `json:"tables"`
	Elements   []models.HallElement  `json:"elements"`
}

func (h *LayoutHandler) Get(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	// The public kiosk route has no wedding-scope middleware, so a missing
	// wedding is answered here, before the table and element queries.
	wedding, err := h.weddingService.Get(ctx, wid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	if err != nil {
		return nil, err
	}
	tables, err := h.tableService.List(ctx, wid)
	if err != nil {
		return nil, err
	}
	elements, err := h.layoutService.Elements(ctx, wid)
	if err != nil {
		return nil, err
	}
	return LayoutResponse{
		HallWidth: wedding.HallWidth, HallHeight: wedding.HallHeight,
		Tables: tables, Elements: elements,
	}, nil
}

func (h *LayoutHandler) Save(c fuego.ContextWithBody[LayoutRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	if body.HallWidth <= 0 || body.HallHeight <= 0 {
		return nil, fuego.BadRequestError{Title: "HallWidth and HallHeight must be > 0"}
	}
	for _, e := range body.Elements {
		if e.X < 0 || e.X > 100 || e.Y < 0 || e.Y > 100 {
			return nil, fuego.BadRequestError{Title: "Element X/Y must be 0–100"}
		}
		if e.Width <= 0 || e.Height <= 0 {
			return nil, fuego.BadRequestError{Title: "Element Width/Height must be > 0"}
		}
		// Column limits of hall_elements: over them Postgres refuses the whole save.
		if err := cmp.Or(maxLen("Element name", e.Name, 255), maxLen("Element color", e.Color, 20),
			maxLen("Element text color", e.TextColor, 20), maxLen("Element stroke color", e.StrokeColor, 20)); err != nil {
			return nil, err
		}
	}
	tables := make([]models.TableUpdate, 0, len(body.Tables))
	for _, t := range body.Tables {
		id, err := middleware.DecodeWIDString(t.ID)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid table id"}
		}
		if t.X < 0 || t.X > 100 || t.Y < 0 || t.Y > 100 {
			return nil, fuego.BadRequestError{Title: "Table X/Y must be 0–100"}
		}
		if t.Capacity != nil && (*t.Capacity < 1 || *t.Capacity > 200) {
			return nil, fuego.BadRequestError{Title: "Table capacity must be between 1 and 200"}
		}
		tables = append(tables, models.TableUpdate{ID: id, X: t.X, Y: t.Y, Degree: t.Degree, Capacity: t.Capacity, IsVip: t.IsVip})
	}
	elements := make([]models.HallElement, 0, len(body.Elements))
	for _, e := range body.Elements {
		if !models.ValidElementType(e.Type) {
			return nil, fuego.BadRequestError{Title: "Invalid element type: " + e.Type}
		}
		el := models.HallElement{
			Type: e.Type, X: e.X, Y: e.Y, Degree: e.Degree,
			Width: e.Width, Height: e.Height, Name: e.Name,
			Color: e.Color, TextColor: e.TextColor, StrokeColor: e.StrokeColor,
			Opacity: e.Opacity, ZIndex: e.ZIndex,
		}
		if e.ID != "" {
			id, err := middleware.DecodeWIDString(e.ID)
			if err != nil {
				return nil, fuego.BadRequestError{Title: "Invalid element id"}
			}
			el.ID = id
		}
		elements = append(elements, el)
	}
	if err := h.layoutService.Save(ctx, wid, body.HallWidth, body.HallHeight, tables, elements); err != nil {
		if errors.Is(err, repository.ErrCapacityBelowSeats) {
			return nil, fuego.ConflictError{Title: "Table capacity is below seats already assigned; move those guests first"}
		}
		// A table or element in this editor was deleted by someone else.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fuego.ConflictError{Title: "The layout changed since you opened it. Reload to see the changes."}
		}
		log.Printf("layout save error: %v", err)
		return nil, fuego.InternalServerError{Title: "Failed to save layout"}
	}
	c.SetStatus(204)
	return nil, nil
}
