package handlers

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

type TableHandler struct{ tableService *services.TableService }

func NewTableHandler(tableService *services.TableService) *TableHandler {
	return &TableHandler{tableService: tableService}
}

type TableRequest struct {
	Name     string  `json:"name"`
	Capacity int     `json:"capacity"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Degree   float64 `json:"degree"`
	IsVip    bool    `json:"isVip"`
}

func (h *TableHandler) List(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	tables, err := h.tableService.List(ctx, wid)
	if err != nil {
		return nil, err
	}
	return tables, nil
}

func (h *TableHandler) Create(c fuego.ContextWithBody[TableRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if err := services.ValidateTableName(body.Name); err != nil {
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	if body.Capacity < 1 || body.Capacity > services.MaxTableCapacity {
		return nil, fuego.BadRequestError{Title: "Capacity must be between 1 and " + strconv.Itoa(services.MaxTableCapacity)}
	}
	table := &models.BanquetTable{
		WeddingID: wid,
		Name:      body.Name,
		Capacity:  body.Capacity,
		X:         body.X,
		Y:         body.Y,
		Degree:    body.Degree,
		IsVip:     body.IsVip,
	}
	if err := h.tableService.Create(ctx, table); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, services.ErrWeddingNotFound) {
			return nil, fuego.NotFoundError{Title: "Wedding not found"}
		}
		slog.ErrorContext(ctx, "create table failed", "wedding", wid, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to create table"}
	}
	c.SetStatus(201)
	return table, nil
}

func (h *TableHandler) Update(c fuego.ContextWithBody[TableRequest]) (any, error) {
	ctx := c.Context()
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	table, err := h.tableService.Get(ctx, id, wid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fuego.NotFoundError{Title: "Table not found"}
	}
	if err != nil {
		slog.ErrorContext(ctx, "load table failed", "table", id, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to load table"}
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if err := services.ValidateTableName(body.Name); err != nil {
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	if body.Capacity < 1 || body.Capacity > services.MaxTableCapacity {
		return nil, fuego.BadRequestError{Title: "Capacity must be between 1 and " + strconv.Itoa(services.MaxTableCapacity)}
	}
	table.Name = body.Name
	table.Capacity = body.Capacity
	table.X = body.X
	table.Y = body.Y
	table.Degree = body.Degree
	table.IsVip = body.IsVip
	if err := h.tableService.Update(ctx, table); err != nil {
		if errors.Is(err, repository.ErrCapacityBelowSeated) {
			return nil, fuego.BadRequestError{Title: err.Error()}
		}
		if errors.Is(err, services.ErrTableNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fuego.NotFoundError{Title: "Table not found"}
		}
		slog.ErrorContext(ctx, "update table failed", "table", id, "err", err)
		return nil, fuego.InternalServerError{Title: "Failed to update table"}
	}
	return table, nil
}

func (h *TableHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	wid, err := DecodeWID(c)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid wedding ID"}
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.tableService.Delete(ctx, id, wid); err != nil {
		if errors.Is(err, services.ErrTableNotFound) {
			return nil, fuego.NotFoundError{Title: "Table not found"}
		}
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}
