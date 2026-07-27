package handlers

import (
	"log"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
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
	HallWidth  int                  `json:"hallWidth"`
	HallHeight int                  `json:"hallHeight"`
	Tables     []models.BanquetTable `json:"tables"`
	Elements   []models.HallElement  `json:"elements"`
}

func (h *LayoutHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	tables, err := h.tableService.List(wid)
	if err != nil {
		return nil, err
	}
	elements, err := h.layoutService.Elements(wid)
	if err != nil {
		return nil, err
	}
	wedding, err := h.weddingService.Get(wid)
	if err != nil {
		return nil, err
	}
	return LayoutResponse{
		HallWidth: wedding.HallWidth, HallHeight: wedding.HallHeight,
		Tables: tables, Elements: elements,
	}, nil
}

func (h *LayoutHandler) Save(c fuego.ContextWithBody[LayoutRequest]) (any, error) {
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
	}
	tablePos := make(map[uuid.UUID][3]float64, len(body.Tables))
	for _, t := range body.Tables {
		id, err := middleware.DecodeWIDString(t.ID)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid table id"}
		}
		if t.X < 0 || t.X > 100 || t.Y < 0 || t.Y > 100 {
			return nil, fuego.BadRequestError{Title: "Table X/Y must be 0–100"}
		}
		tablePos[id] = [3]float64{t.X, t.Y, t.Degree}
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
	if err := h.layoutService.Save(wid, body.HallWidth, body.HallHeight, tablePos, elements); err != nil {
		log.Printf("layout save error: %v", err)
		return nil, fuego.InternalServerError{Title: "Failed to save layout"}
	}
	return nil, nil
}
