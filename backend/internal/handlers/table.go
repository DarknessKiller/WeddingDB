package handlers

import (
	"sort"

	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
)

type TableHandler struct{ tableService *services.TableService }

func NewTableHandler(tableService *services.TableService) *TableHandler {
	return &TableHandler{tableService: tableService}
}

type TableRequest struct {
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	IsVip    bool `json:"isVip"`
}

type TableResponse struct {
	ID        uint    `json:"id"`
	WeddingID uint    `json:"weddingId"`
	Name      string  `json:"name"`
	Capacity  int     `json:"capacity"`
	Row       int     `json:"row"`
	Col       int     `json:"col"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	IsVip     bool `json:"isVip"`
}

var yPositions = map[int]float64{1: 15, 2: 30, 3: 45, 4: 60, 5: 75, 6: 90}

func computeLayout(tables []models.BanquetTable) []TableResponse {
	type twp struct {
		models.BanquetTable
		x, y float64
	}
	rowMap := make(map[int][]*twp)
	for i := range tables {
		rowMap[tables[i].Row] = append(rowMap[tables[i].Row], &twp{BanquetTable: tables[i]})
	}

	// Find max row number
	maxRow := 0
	for row := range rowMap {
		if row > maxRow {
			maxRow = row
		}
	}

	// If more rows than predefined, compute positions dynamically
	yPos := make(map[int]float64)
	if maxRow <= len(yPositions) {
		for k, v := range yPositions {
			yPos[k] = v
		}
	} else {
		// Evenly space rows with good gaps
		start, end := 12.0, 88.0
		for i := 1; i <= maxRow; i++ {
			if maxRow == 1 {
				yPos[i] = (start + end) / 2
			} else {
				yPos[i] = start + (end-start)*float64(i-1)/float64(maxRow-1)
			}
		}
	}
	var result []TableResponse
	for row, rowTables := range rowMap {
		sort.Slice(rowTables, func(i, j int) bool {
			return rowTables[i].Col < rowTables[j].Col
		})
		n := len(rowTables)
		y := yPos[row]
		if y == 0 {
			y = 50
		}
		for i, t := range rowTables {
			t.x = float64(100) / float64(n+1) * float64(i+1)
			t.y = y
			result = append(result, TableResponse{
				ID: t.ID, WeddingID: t.WeddingID, Name: t.Name, Capacity: t.Capacity,
				Row: t.Row, Col: t.Col, X: t.x, Y: t.y, IsVip: t.IsVip,
			})
		}
	}
	return result
}

func (h *TableHandler) List(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	tables, err := h.tableService.List(wid)
	if err != nil {
		return nil, err
	}
	return computeLayout(tables), nil
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
		Row:       body.Row,
		Col:       body.Col,
		IsVip:     body.IsVip,
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
	table.Row = body.Row
	table.Col = body.Col
	table.IsVip = body.IsVip
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
