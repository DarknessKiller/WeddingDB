package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type TableHandler struct{ tableService *services.TableService }

func NewTableHandler(tableService *services.TableService) *TableHandler {
	return &TableHandler{tableService: tableService}
}

type TableRequest struct {
	Name     string  `json:"n"`
	Capacity int     `json:"cap"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	IsVip    bool    `json:"v"`
	Zone     string  `json:"z"`
}

func (h *TableHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	tables, err := h.tableService.List(wid)
	if err != nil {
		return nil, err
	}
	return tables, nil
}

func (h *TableHandler) Create(c fuego.ContextWithBody[TableRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
	table := &models.BanquetTable{
		WeddingID: wid,
		Name:      body.Name,
		Capacity:  body.Capacity,
		X:         body.X,
		Y:         body.Y,
		IsVip:     body.IsVip,
		Zone:      body.Zone,
	}
	if err := h.tableService.Create(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (h *TableHandler) Update(c fuego.ContextWithBody[TableRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	table, err := h.tableService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Table not found"}
	}
	table.Name = body.Name
	table.Capacity = body.Capacity
	table.X = body.X
	table.Y = body.Y
	table.IsVip = body.IsVip
	table.Zone = body.Zone
	if err := h.tableService.Update(table); err != nil {
		return nil, err
	}
	return table, nil
}

func (h *TableHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.tableService.Delete(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}
