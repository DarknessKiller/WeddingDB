# Hall Map Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace fixed row/col banquet layout with a fully customizable per-wedding hall map (tables + stage/DJ/entrance/TV/walkway/box elements) edited via drag/rotate/resize on a Konva canvas.

**Architecture:** Backend stores x/y/degree verbatim (no layout math), exposes one atomic `PATCH /layout` plus public/auth `GET /layout`. Frontend renders one shared `svelte-konva` `HallMap` component in `view` mode (kiosk, search, reservation) and `edit` mode (tables + seating pages). Spec: `docs/superpowers/specs/2026-07-26-hall-map-editor-design.md`.

**Tech Stack:** Go 1.26, gorm/postgres, fuego · SvelteKit 2 + Svelte 5 (runes), Tailwind 4, konva + svelte-konva, vitest (new dev dep)

## Global Constraints

- Work in worktree `.worktrees/hall-map-editor-20260726-233631`, branch `hall-map-editor`.
- Conventional Commits.
- Positions are percentages (0–100) of the wedding's hall canvas; element x/y is the element **center** (same as tables).
- Element types: `stage | dj_counter | entrance | tv | walkway | box`.
- Backend runs via `cd backend && go build ./... && go test ./...`.
- Frontend check: `cd frontend && npm run check`. Frontend tests: `npx vitest run`.
- No `row`/`col` anywhere after this plan (DB columns, API, types, UI).
- Migrations follow the repo's existing pattern: `db.AutoMigrate` + idempotent bootstrap backfill (see `bootstrap.go` "ponytail" backfills), plus explicit `Migrator().DropColumn` for row/col.

---

### Task 1: Backend models + row/col→x/y backfill math

**Files:**
- Modify: `backend/internal/models/banquet_table.go`
- Modify: `backend/internal/models/wedding_event.go`
- Create: `backend/internal/models/hall_element.go`
- Create: `backend/internal/models/layout.go`
- Test: `backend/internal/models/layout_test.go`

**Interfaces:**
- Produces (used by Tasks 2–4, and mirrored in frontend Task 5):
  - `models.BanquetTable{X, Y, Degree float64}` (Row/Col removed)
  - `models.HallElement{ID, WeddingID uuid.UUID; Type string; X, Y, Degree, Width, Height float64; Label string; ZIndex int}`
  - `models.WeddingEvent{HallWidth, HallHeight int}` (defaults 860/1000)
  - `models.ElementTypes = []string{"stage","dj_counter","entrance","tv","walkway","box"}`
  - `models.RowColToXY(tables []BanquetTable) map[uuid.UUID][2]float64` — pure backfill math
  - `models.DefaultElements(weddingID uuid.UUID) []HallElement`

- [ ] **Step 1: Write failing test** `backend/internal/models/layout_test.go`

```go
package models

import (
	"testing"

	"github.com/google/uuid"
)

func tablesFromRowCol(rc ...[2]int) []BanquetTable {
	out := make([]BanquetTable, len(rc))
	for i, p := range rc {
		out[i] = BanquetTable{ID: uuid.New(), Row: p[0], Col: p[1]}
	}
	return out
}

func TestRowColToXY_FixedYPositions(t *testing.T) {
	tables := tablesFromRowCol([2]int{1, 1}, [2]int{1, 2}, [2]int{3, 1})
	got := RowColToXY(tables)
	// yPositions: row1=15, row3=45; maxCol=2 -> x = 100/3*col
	if got[tables[0].ID] != [2]float64{100.0 / 3.0, 15} {
		t.Errorf("row1col1: got %v", got[tables[0].ID])
	}
	if got[tables[1].ID] != [2]float64{100.0 / 3.0 * 2, 15} {
		t.Errorf("row1col2: got %v", got[tables[1].ID])
	}
	if got[tables[2].ID] != [2]float64{100.0 / 3.0, 45} {
		t.Errorf("row3col1: got %v", got[tables[2].ID])
	}
}

func TestRowColToXY_BeyondSixRows(t *testing.T) {
	tables := tablesFromRowCol([2]int{1, 1}, [2]int{7, 1})
	got := RowColToXY(tables)
	// maxRow=7 > 6 -> linear spread 12..88: row1=12, row7=88
	if got[tables[0].ID][1] != 12 {
		t.Errorf("row1 y: got %v", got[tables[0].ID][1])
	}
	if got[tables[1].ID][1] != 88 {
		t.Errorf("row7 y: got %v", got[tables[1].ID][1])
	}
}

func TestDefaultElements(t *testing.T) {
	wid := uuid.New()
	els := DefaultElements(wid)
	if len(els) != 5 {
		t.Fatalf("want 5 default elements, got %d", len(els))
	}
	counts := map[string]int{}
	for _, e := range els {
		if e.WeddingID != wid {
			t.Errorf("wrong wedding id")
		}
		counts[e.Type]++
	}
	if counts["stage"] != 1 || counts["entrance"] != 1 || counts["walkway"] != 3 {
		t.Errorf("bad defaults: %v", counts)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd backend && go test ./internal/models/ -run 'RowColToXY|DefaultElements' -v`
Expected: FAIL (undefined: RowColToXY)

- [ ] **Step 3: Implement**

`backend/internal/models/banquet_table.go` — replace Row/Col with X/Y/Degree:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type BanquetTable struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Capacity  int       `gorm:"not null" json:"capacity"`
	X         float64   `gorm:"not null;default:0" json:"x"`
	Y         float64   `gorm:"not null;default:0" json:"y"`
	Degree    float64   `gorm:"not null;default:0" json:"degree"`
	IsVip     bool      `json:"isVip"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
```

Add to `wedding_event.go` struct:

```go
	HallWidth           int       `gorm:"not null;default:860" json:"hallWidth"`
	HallHeight          int       `gorm:"not null;default:1000" json:"hallHeight"`
```

`backend/internal/models/hall_element.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

var ElementTypes = []string{"stage", "dj_counter", "entrance", "tv", "walkway", "box"}

func ValidElementType(t string) bool {
	for _, v := range ElementTypes {
		if v == t {
			return true
		}
	}
	return false
}

type HallElement struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID uuid.UUID `gorm:"type:uuid;index;not null" json:"weddingId"`
	Type      string    `gorm:"size:20;not null" json:"type"`
	X         float64   `gorm:"not null;default:0" json:"x"`
	Y         float64   `gorm:"not null;default:0" json:"y"`
	Degree    float64   `gorm:"not null;default:0" json:"degree"`
	Width     float64   `gorm:"not null;default:0" json:"width"`
	Height    float64   `gorm:"not null;default:0" json:"height"`
	Label     string    `gorm:"size:255" json:"label"`
	ZIndex    int       `gorm:"not null;default:0" json:"zIndex"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
```

`backend/internal/models/layout.go` — backfill math mirrors old `computeLayout` exactly, plus default elements (x/y = center, % of canvas):

```go
package models

import "github.com/google/uuid"

var yPositions = map[int]float64{1: 15, 2: 30, 3: 45, 4: 60, 5: 75, 6: 90}

// RowColToXY computes x/y percentages from legacy row/col values.
// Reads the legacy Row/Col fields still present in the DB row at migration time.
func RowColToXY(tables []BanquetTable) map[uuid.UUID][2]float64 {
	maxRow, maxCol := 0, 0
	for _, t := range tables {
		if t.Row > maxRow {
			maxRow = t.Row
		}
		if t.Col > maxCol {
			maxCol = t.Col
		}
	}
	if maxCol == 0 {
		maxCol = 3
	}
	yPos := make(map[int]float64)
	if maxRow <= len(yPositions) {
		for k, v := range yPositions {
			yPos[k] = v
		}
	} else {
		start, end := 12.0, 88.0
		for i := 1; i <= maxRow; i++ {
			if maxRow == 1 {
				yPos[i] = (start + end) / 2
			} else {
				yPos[i] = start + (end-start)*float64(i-1)/float64(maxRow-1)
			}
		}
	}
	out := make(map[uuid.UUID][2]float64, len(tables))
	for _, t := range tables {
		y, ok := yPos[t.Row]
		if !ok || y == 0 {
			y = 50
		}
		out[t.ID] = [2]float64{100.0 / float64(maxCol+1) * float64(t.Col), y}
	}
	return out
}

// DefaultElements reproduces the previously hardcoded HallMap decorations.
func DefaultElements(weddingID uuid.UUID) []HallElement {
	mk := func(typ string, x, y, w, h float64, label string, z int) HallElement {
		return HallElement{WeddingID: weddingID, Type: typ, X: x, Y: y, Width: w, Height: h, Label: label, ZIndex: z}
	}
	return []HallElement{
		mk("stage", 50, 3, 55, 6, "✦ Stage ✦", 10),
		mk("entrance", 50, 98, 14, 4, "▼ Entrance ▼", 10),
		mk("walkway", 50, 50, 0.3, 84, "", 1),
		mk("walkway", 30, 50, 0.3, 84, "", 1),
		mk("walkway", 70, 50, 0.3, 84, "", 1),
	}
}
```

NOTE: `RowColToXY` references `t.Row`/`t.Col`, which Step 3 just removed from the struct. Keep legacy fields privately: add to `layout.go`:

```go
// legacyRowCol mirrors the dropped row/col DB columns for migration reads.
type legacyRowCol struct {
	ID  uuid.UUID
	Row int
	Col int
}
```

and change `RowColToXY` signature to accept `[]legacyRowCol`. Adjust test: `tablesFromRowCol` returns `[]legacyRowCol` constructed via an exported constructor `LegacyRowCol(id uuid.UUID, row, col int) legacyRowCol`... 

SIMPLER (do this): keep exported pure function operating on plain values:

```go
// RowColToXY computes x/y percentages from legacy row/col values.
// rows[i], cols[i] pair per table; ids[i] identifies the table.
func RowColToXY(ids []uuid.UUID, rows, cols []int) map[uuid.UUID][2]float64
```

Test becomes:

```go
ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
got := RowColToXY(ids, []int{1, 1, 3}, []int{1, 2, 1})
if got[ids[0]] != [2]float64{100.0 / 3.0, 15} { ... }
```

(Implement the function body shown above with rows/cols slices instead of struct fields.)

- [ ] **Step 4: Run, verify pass**

Run: `cd backend && go test ./internal/models/ -run 'RowColToXY|DefaultElements' -v && go build ./...`
Expected: PASS; build FAILS in `handlers/table.go` (Row/Col references) — that is Task 3's job; for now only `go vet ./internal/models` must pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models
git commit -m "feat(backend): x/y/degree table model, hall elements, backfill math"
```

---

### Task 2: Bootstrap migration — backfill, seed elements, drop row/col

**Files:**
- Modify: `backend/internal/bootstrap/bootstrap.go:52-82`

**Interfaces:**
- Consumes: `models.RowColToXY`, `models.DefaultElements`, `models.HallElement`, `models.BanquetTable`.
- Produces: idempotent startup migration (safe to re-run).

- [ ] **Step 1: Implement migration block** in `bootstrap.go` immediately after the existing `db.AutoMigrate(...)` call. Add `&models.HallElement{}` to the AutoMigrate list, then:

```go
	// ponytail: one-time row/col -> x/y backfill + default hall elements (idempotent)
	if db.Migrator().HasColumn(&models.BanquetTable{}, "Row") {
		type legacyTable struct {
			ID  uuid.UUID
			Row int
			Col int
		}
		var weddingIDs []uuid.UUID
		db.Model(&models.BanquetTable{}).Distinct().Pluck("wedding_id", &weddingIDs)
		for _, wid := range weddingIDs {
			var rows []legacyTable
			db.Table("banquet_tables").Select("id, row, col").Where("wedding_id = ?", wid).Scan(&rows)
			ids := make([]uuid.UUID, len(rows))
			r := make([]int, len(rows))
			c := make([]int, len(rows))
			for i, t := range rows {
				ids[i], r[i], c[i] = t.ID, t.Row, t.Col
			}
			for id, pos := range models.RowColToXY(ids, r, c) {
				db.Model(&models.BanquetTable{}).Where("id = ?", id).Updates(map[string]any{"x": pos[0], "y": pos[1]})
			}
		}
		if err := db.Migrator().DropColumn(&models.BanquetTable{}, "Row"); err != nil {
			log.Println("Warning: drop row column:", err)
		}
		if err := db.Migrator().DropColumn(&models.BanquetTable{}, "Col"); err != nil {
			log.Println("Warning: drop col column:", err)
		}
	}
	// Seed default elements for weddings that have none
	var allWeddings []uuid.UUID
	db.Model(&models.WeddingEvent{}).Pluck("id", &allWeddings)
	for _, wid := range allWeddings {
		var n int64
		db.Model(&models.HallElement{}).Where("wedding_id = ?", wid).Count(&n)
		if n == 0 {
			db.Create(models.DefaultElements(wid))
		}
	}
```

`models.BanquetTable` no longer has Row/Col, so `HasColumn(&models.BanquetTable{}, "Row")` inspects DB schema — correct (gorm checks the DB, not the struct).

- [ ] **Step 2: Verify build + manual run**

Run: `cd backend && go build ./...` — expect failure only in `handlers/table.go` (fixed in Task 3).
Then (after Task 3 completes, or temporarily commenting nothing — defer to integration): `docker compose up -d && cd backend && go run ./cmd/server`, check logs for clean startup, and `psql $DATABASE_URL -c 'select x,y,degree from banquet_tables limit 5; select type,x,y from hall_elements limit 5;'`
Expected: x/y match old grid values; 5 seeded elements per wedding.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/bootstrap/bootstrap.go
git commit -m "feat(backend): migrate row/col to x/y, seed default hall elements"
```

---

### Task 3: Table handler — drop computeLayout, x/y/degree passthrough

**Files:**
- Modify: `backend/internal/handlers/table.go` (rewrite)

**Interfaces:**
- Consumes: `models.BanquetTable` (X/Y/Degree).
- Produces: `TableRequest{Name string; Capacity int; X, Y, Degree float64; IsVip bool}`; `TableResponse` = same plus ID/WeddingID. JSON field names unchanged from spec (`x`, `y`, `degree`).

- [ ] **Step 1: Rewrite `table.go`** — delete `computeLayout`, `yPositions`, `sort` import. Since the model now matches the response shape, return the model directly:

```go
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
	Name     string  `json:"name"`
	Capacity int     `json:"capacity"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Degree   float64 `json:"degree"`
	IsVip    bool    `json:"isVip"`
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
		Degree:    body.Degree,
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
	table.X = body.X
	table.Y = body.Y
	table.Degree = body.Degree
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
```

- [ ] **Step 2: Build**

Run: `cd backend && go build ./...`
Expected: PASS (whole backend compiles again).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/table.go
git commit -m "refactor(backend): tables store x/y/degree, drop computeLayout"
```

---

### Task 4: Layout API — GET (auth + public) and PATCH

**Files:**
- Create: `backend/internal/handlers/layout.go`
- Create: `backend/internal/services/layout_service.go`
- Create: `backend/internal/repository/layout_repo.go`
- Create: `backend/internal/services/layout_service_test.go`
- Modify: `backend/internal/handlers/register.go`
- Modify: `backend/internal/bootstrap/bootstrap.go` (wire repo/service)

**Interfaces:**
- Consumes: models from Task 1; `weddingRepo` pattern from `repository/wedding_repo.go`.
- Produces:
  - `GET /api/weddings/{wid}/layout` and `GET /api/public/weddings/{wid}/layout` → `LayoutResponse{HallWidth, HallHeight int; Tables []models.BanquetTable; Elements []models.HallElement}`
  - `PATCH /api/weddings/{wid}/layout` accepting `LayoutRequest{HallWidth, HallHeight int; Tables []TablePos{ID string; X, Y, Degree float64}; Elements []ElementInput{ID string; Type string; X, Y, Degree, Width, Height float64; Label string; ZIndex int}}`
  - `services.ReconcileElements(existing []models.HallElement, incoming []models.HallElement) (toCreate, toUpdate []models.HallElement, toDelete []uuid.UUID)` — pure, unit-tested.

- [ ] **Step 1: Write failing test** `backend/internal/services/layout_service_test.go`

```go
package services

import (
	"testing"

	"github.com/google/uuid"
	"weddingdb/internal/models"
)

func TestReconcileElements(t *testing.T) {
	wid := uuid.New()
	keep := models.HallElement{ID: uuid.New(), WeddingID: wid, Type: "stage"}
	drop := models.HallElement{ID: uuid.New(), WeddingID: wid, Type: "tv"}
	existing := []models.HallElement{keep, drop}

	updated := keep
	updated.X = 42
	fresh := models.HallElement{WeddingID: wid, Type: "box", X: 10, Y: 10, Width: 20, Height: 20}
	incoming := []models.HallElement{updated, fresh}

	toCreate, toUpdate, toDelete := ReconcileElements(existing, incoming)
	if len(toCreate) != 1 || toCreate[0].Type != "box" {
		t.Errorf("create: %+v", toCreate)
	}
	if len(toUpdate) != 1 || toUpdate[0].ID != keep.ID || toUpdate[0].X != 42 {
		t.Errorf("update: %+v", toUpdate)
	}
	if len(toDelete) != 1 || toDelete[0] != drop.ID {
		t.Errorf("delete: %+v", toDelete)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd backend && go test ./internal/services/ -run ReconcileElements -v`
Expected: FAIL (undefined: ReconcileElements)

- [ ] **Step 3: Implement**

`backend/internal/repository/layout_repo.go`:

```go
package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type LayoutRepo struct{ db *gorm.DB }

func NewLayoutRepo(db *gorm.DB) *LayoutRepo { return &LayoutRepo{db: db} }

func (r *LayoutRepo) ElementsByWedding(weddingID uuid.UUID) ([]models.HallElement, error) {
	els := make([]models.HallElement, 0)
	err := r.db.Where("wedding_id = ?", weddingID).Order("z_index").Find(&els).Error
	return els, err
}

// SaveLayout applies table positions, element reconcile, and hall size in one transaction.
func (r *LayoutRepo) SaveLayout(
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tablePos map[uuid.UUID][3]float64,
	toCreate, toUpdate []models.HallElement,
	toDelete []uuid.UUID,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if hallWidth > 0 && hallHeight > 0 {
			if err := tx.Model(&models.WeddingEvent{}).Where("id = ?", weddingID).
				Updates(map[string]any{"hall_width": hallWidth, "hall_height": hallHeight}).Error; err != nil {
				return err
			}
		}
		for id, pos := range tablePos {
			res := tx.Model(&models.BanquetTable{}).Where("id = ? AND wedding_id = ?", id, weddingID).
				Updates(map[string]any{"x": pos[0], "y": pos[1], "degree": pos[2]})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		for i := range toCreate {
			toCreate[i].WeddingID = weddingID
			if err := tx.Create(&toCreate[i]).Error; err != nil {
				return err
			}
		}
		for i := range toUpdate {
			res := tx.Model(&models.HallElement{}).Where("id = ? AND wedding_id = ?", toUpdate[i].ID, weddingID).
				Updates(map[string]any{
					"type": toUpdate[i].Type, "x": toUpdate[i].X, "y": toUpdate[i].Y,
					"degree": toUpdate[i].Degree, "width": toUpdate[i].Width, "height": toUpdate[i].Height,
					"label": toUpdate[i].Label, "z_index": toUpdate[i].ZIndex,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		if len(toDelete) > 0 {
			if err := tx.Where("id IN ? AND wedding_id = ?", toDelete, weddingID).
				Delete(&models.HallElement{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
```

`backend/internal/services/layout_service.go`:

```go
package services

import (
	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

type LayoutService struct {
	layoutRepo *repository.LayoutRepo
}

func NewLayoutService(lr *repository.LayoutRepo) *LayoutService {
	return &LayoutService{layoutRepo: lr}
}

func (s *LayoutService) Elements(weddingID uuid.UUID) ([]models.HallElement, error) {
	return s.layoutRepo.ElementsByWedding(weddingID)
}

func (s *LayoutService) Save(
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tablePos map[uuid.UUID][3]float64,
	incoming []models.HallElement,
) error {
	existing, err := s.layoutRepo.ElementsByWedding(weddingID)
	if err != nil {
		return err
	}
	toCreate, toUpdate, toDelete := ReconcileElements(existing, incoming)
	return s.layoutRepo.SaveLayout(weddingID, hallWidth, hallHeight, tablePos, toCreate, toUpdate, toDelete)
}

// ReconcileElements diffs desired elements against stored ones (full-replace semantics).
// Incoming elements without an ID are creates; with an ID are updates;
// existing elements absent from incoming are deletes.
func ReconcileElements(existing, incoming []models.HallElement) (toCreate, toUpdate []models.HallElement, toDelete []uuid.UUID) {
	incomingIDs := make(map[uuid.UUID]bool, len(incoming))
	for _, e := range incoming {
		if e.ID == uuid.Nil {
			toCreate = append(toCreate, e)
		} else {
			toUpdate = append(toUpdate, e)
			incomingIDs[e.ID] = true
		}
	}
	for _, e := range existing {
		if !incomingIDs[e.ID] {
			toDelete = append(toDelete, e.ID)
		}
	}
	return toCreate, toUpdate, toDelete
}
```

`backend/internal/handlers/layout.go`:

```go
package handlers

import (
	"weddingdb/internal/models"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

type LayoutHandler struct {
	layoutService   *services.LayoutService
	tableService    *services.TableService
	weddingService  *services.WeddingService
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
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Degree float64 `json:"degree"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Label  string  `json:"label"`
	ZIndex int     `json:"zIndex"`
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
	wid := DecodeWID(c)
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
	wid := DecodeWID(c)
	tablePos := make(map[uuid.UUID][3]float64, len(body.Tables))
	for _, t := range body.Tables {
		id, err := uuid.Parse(t.ID)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid table id"}
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
			Width: e.Width, Height: e.Height, Label: e.Label, ZIndex: e.ZIndex,
		}
		if e.ID != "" {
			id, err := uuid.Parse(e.ID)
			if err != nil {
				return nil, fuego.BadRequestError{Title: "Invalid element id"}
			}
			el.ID = id
		}
		elements = append(elements, el)
	}
	if err := h.layoutService.Save(wid, body.HallWidth, body.HallHeight, tablePos, elements); err != nil {
		return nil, fuego.BadRequestError{Title: "Failed to save layout: " + err.Error()}
	}
	return nil, nil
}
```

`weddingService.Get(id uuid.UUID) (*models.WeddingEvent, error)` — confirmed in `services/wedding_service.go:21`.

Wire in `bootstrap.go`:

```go
	layoutRepo := repository.NewLayoutRepo(db)
	layoutService := services.NewLayoutService(layoutRepo)
```

and pass `layoutService` into `handlers.RegisterRoutes(...)`.

Wire in `register.go` — add param `layoutService *services.LayoutService`, create handler, register:

```go
	layoutHandler := NewLayoutHandler(layoutService, tableService, weddingService)
	// public:
	fuego.Get(pubScoped, "/layout", layoutHandler.Get)
	// scoped (after WeddingScopeMiddleware):
	fuego.Get(scoped, "/layout", layoutHandler.Get)
	fuego.Patch(scoped, "/layout", layoutHandler.Save)
```

NOTE: public `Get` calls `DecodeWID` — verify `DecodeWID` handles the public path param (public tables route already uses `tableHandler.List` with `DecodeWID`, so it works).

- [ ] **Step 4: Run tests + build**

Run: `cd backend && go test ./internal/services/ -run ReconcileElements -v && go build ./...`
Expected: PASS + build OK.

- [ ] **Step 5: Manual smoke**

Run server against dev DB, then:
```bash
curl -s localhost:8080/api/public/weddings/<WID>/layout | jq '.hallWidth, (.elements|length)'
```
Expected: 860 and 5.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/layout.go backend/internal/services/layout_service.go backend/internal/services/layout_service_test.go backend/internal/repository/layout_repo.go backend/internal/handlers/register.go backend/internal/bootstrap/bootstrap.go
git commit -m "feat(backend): layout GET/PATCH endpoints with element reconcile"
```

---

### Task 5: Frontend deps, types, API client, defaultSlot util

**Files:**
- Modify: `frontend/package.json` (add `konva`, `svelte-konva`; dev dep `vitest`)
- Modify: `frontend/src/lib/types/index.ts`
- Create: `frontend/src/lib/api/layout.ts`
- Create: `frontend/src/lib/utils/layout.ts`
- Test: `frontend/src/lib/utils/layout.test.ts`

**Interfaces:**
- Produces (used by Tasks 6–8):
  - `interface HallElement { id: string; type: ElementType; x: number; y: number; degree: number; width: number; height: number; label: string; zIndex: number }`
  - `type ElementType = 'stage' | 'dj_counter' | 'entrance' | 'tv' | 'walkway' | 'box'`
  - `interface HallLayoutData { hallWidth: number; hallHeight: number; tables: BanquetTable[]; elements: HallElement[] }`
  - `BanquetTable` gains `degree: number`, loses `row`/`col`
  - `getLayout(wid: string): Promise<HallLayoutData>`, `saveLayout(wid: string, data: SaveLayoutPayload): Promise<void>`, `getPublicLayout(wid: string): Promise<HallLayoutData>`
  - `defaultSlot(tables: BanquetTable[]): { x: number; y: number }`
  - `createTable`/`updateTable` payloads now include `x`, `y`, `degree` (no row/col)

- [ ] **Step 1: Install deps**

Run: `cd frontend && npm install konva svelte-konva && npm install -D vitest`
Expected: added to package.json. If `svelte-konva` has peer issues with Svelte 5, use `npm install svelte-konva --legacy-peer-deps` and note it in the commit message.

- [ ] **Step 2: Write failing test** `frontend/src/lib/utils/layout.test.ts`

```ts
import { describe, it, expect } from 'vitest';
import { defaultSlot } from './layout';
import type { BanquetTable } from '$lib/types';

const t = (x: number, y: number): BanquetTable => ({
	id: crypto.randomUUID(), name: '', capacity: 10, x, y, degree: 0, isVip: false
});

describe('defaultSlot', () => {
	it('returns first slot when empty', () => {
		expect(defaultSlot([])).toEqual({ x: 25, y: 15 });
	});
	it('advances along the row', () => {
		expect(defaultSlot([t(25, 15)])).toEqual({ x: 50, y: 15 });
	});
	it('wraps to next row after 5 columns', () => {
		const tables = [t(25, 15), t(50, 15), t(75, 15), t(100, 15), t(125, 15)];
		expect(defaultSlot(tables)).toEqual({ x: 25, y: 30 });
	});
});
```

NOTE: old grid x used `100/(maxCol+1)*col` which drifts as columns grow; the default slot only needs to look right for new tables, so we use a fixed 5-column grid (`x = 25*col`, `y = 15*(row)`, row/col 1-based, 5 cols per row). This intentionally differs from the old drift-y math — `ponytail: fixed 5-col default grid; drag exists for everything else`.

- [ ] **Step 3: Run, verify fail**

Run: `cd frontend && npx vitest run src/lib/utils/layout.test.ts`
Expected: FAIL (Cannot find module './layout')

- [ ] **Step 4: Implement**

`frontend/src/lib/utils/layout.ts`:

```ts
import type { BanquetTable } from '$lib/types';

// ponytail: fixed 5-col default grid; drag exists for everything else
export function defaultSlot(tables: BanquetTable[]): { x: number; y: number } {
	const taken = new Set(tables.map((t) => `${Math.round(t.x)},${Math.round(t.y)}`));
	for (let row = 1; row <= 20; row++) {
		for (let col = 1; col <= 5; col++) {
			const x = (100 / 6) * col;
			const y = 15 * row;
			if (!taken.has(`${Math.round(x)},${Math.round(y)}`)) {
				return { x, y };
			}
		}
	}
	return { x: 50, y: 50 };
}
```

Adjust test expectations: `100/6*1 ≈ 16.67`, so first slot is `{ x: 100/6, y: 15 }`. Write test accordingly (`toBeCloseTo`).

`frontend/src/lib/types/index.ts` — replace `BanquetTable` and `HallLayout`, add element types:

```ts
export interface BanquetTable {
	id: string;
	name: string;
	capacity: number;
	x: number;
	y: number;
	degree: number;
	isVip: boolean;
}

export type ElementType = 'stage' | 'dj_counter' | 'entrance' | 'tv' | 'walkway' | 'box';

export interface HallElement {
	id: string;
	type: ElementType;
	x: number;
	y: number;
	degree: number;
	width: number;
	height: number;
	label: string;
	zIndex: number;
}

export interface HallLayoutData {
	hallWidth: number;
	hallHeight: number;
	tables: BanquetTable[];
	elements: HallElement[];
}
```

(Delete old `HallLayout` interface.)

`frontend/src/lib/api/layout.ts`:

```ts
import { apiFetch } from './client';
import type { HallLayoutData, HallElement } from '$lib/types';

export async function getLayout(wid: string): Promise<HallLayoutData> {
	const res = await apiFetch(`/api/weddings/${wid}/layout`);
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

export async function getPublicLayout(wid: string): Promise<HallLayoutData> {
	const res = await fetch(`/api/public/weddings/${wid}/layout`);
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

export interface SaveLayoutPayload {
	hallWidth: number;
	hallHeight: number;
	tables: { id: string; x: number; y: number; degree: number }[];
	elements: Omit<HallElement, 'weddingId'>[];
}

export async function saveLayout(wid: string, data: SaveLayoutPayload): Promise<void> {
	const res = await apiFetch(`/api/weddings/${wid}/layout`, {
		method: 'PATCH',
		body: JSON.stringify(data)
	});
	if (!res.ok) throw new Error('Failed to save layout');
}
```

`frontend/src/lib/api/tables.ts` — change `createTable`/`updateTable` signatures from `Omit<BanquetTable, 'id' | 'x' | 'y'>` to `Omit<BanquetTable, 'id'>`.

- [ ] **Step 5: Run test, verify pass**

Run: `cd frontend && npx vitest run src/lib/utils/layout.test.ts`
Expected: PASS. (`npm run check` will fail until Task 6/7 fix call sites — expected.)

- [ ] **Step 6: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/src/lib/types/index.ts frontend/src/lib/api/layout.ts frontend/src/lib/api/tables.ts frontend/src/lib/utils/layout.ts frontend/src/lib/utils/layout.test.ts
git commit -m "feat(frontend): layout api client, hall element types, defaultSlot util"
```

---

### Task 6: Konva HallMap — view mode

**Files:**
- Create: `frontend/src/lib/components/seating/HallElementNode.svelte`
- Rewrite: `frontend/src/lib/components/seating/BanquetTable.svelte`
- Rewrite: `frontend/src/lib/components/seating/HallMap.svelte`
- Modify: `frontend/src/routes/kiosk/[wid]/+page.svelte` (use `getPublicLayout`, pass elements)
- Modify: `frontend/src/routes/[wid]/search/+page.svelte`
- Modify: `frontend/src/routes/[wid]/reservation/+page.svelte`

**Interfaces:**
- Consumes: Task 5 types + `getLayout`/`getPublicLayout`.
- Produces: `HallMap` props:
  ```ts
  {
    mode?: 'view' | 'edit';            // default 'view'
    tables: BanquetTable[];
    elements: HallElement[];
    hallWidth: number; hallHeight: number;
    tableGuests?: Record<string, Guest[]>;
    selectedTableId?: string | null;
    highlightedTableId?: string | null;
    dark?: boolean;
    onTableClick?: (id: string) => void;
    onSeatClick?: (tableId: string, seatNum: number, guest: Guest | null) => void;
    // edit mode only (Task 7):
    editTables?: BanquetTable[];       // bindable working copies
    editElements?: HallElement[];
    onSaveLayout?: () => void;
  }
  ```
  `BanquetTable` becomes a Konva `Group` component (named export shape unchanged for callers: same props as before plus `degree`).

**SSR constraint:** Konva is browser-only. Pattern (use exactly this):

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  let KonvaStage: any = $state(null);
  let Layer: any = $state(null);
  onMount(async () => {
    const mod = await import('svelte-konva');
    KonvaStage = mod.Stage;
    Layer = mod.Layer;
  });
</script>

{#if KonvaStage && Layer}
  <KonvaStage ...><Layer>...</Layer></KonvaStage>
{/if}
```

- [ ] **Step 1: Rewrite `BanquetTable.svelte` as Konva group**

Same visual structure as current SVG, as Konva primitives (`Circle`, `Arc`, `Text`, `Group`). Props: existing props + `scale` (canvas unit multiplier). Key mapping:

- outer div → `<Group x={px} y={py} rotation={table.degree} draggable={mode==='edit'}>`
- table circle → `<Circle radius={36} fillRadialGradient...>`
- occupancy ring → `<Arc innerRadius={37} outerRadius={40} angle={360*pct} rotation={-90} fill={color}>`
- name/pax → `<Text>`
- seats → orbit `Circle`s at `ORBIT_RADIUS`, seat i angle `(360*i/capacity - 90)` (identical math to current `seatPos`)
- seat click: `on:click`/`on:tap` → `onSeatClick?.(i+1, guest)` (only in view mode)
- group click: `onTableClick?.()` (only in view mode)

Position: caller passes canvas pixels: `x = table.x/100 * hallWidth`, `y = table.y/100 * hallHeight`.

- [ ] **Step 2: Create `HallElementNode.svelte`**

Konva `Group` (x/y center, rotation=degree) containing a `Rect` (offsetX/offsetY = w/2, h/2) + optional `Text` label. Style per type:

| type | fill | stroke | text |
|---|---|---|---|
| stage | `#A11217` gradient → plain `#7F1D1D` | `#D4AF37` | label, gold |
| dj_counter | `#1F2937` | `#4B5563` | label, white |
| entrance | `#E5E7EB` | `#9CA3AF` | label, gray |
| tv | `#111827` | `#374151` | 'TV' |
| walkway | `#374151` | none | none |
| box | transparent | `#1F2937` 2px | label top-left |

w/h in canvas px: `width/100 * hallWidth`, `height/100 * hallHeight`.

- [ ] **Step 3: Rewrite `HallMap.svelte`**

Keep existing zoom/pan container div + zoom controls + legend (unchanged DOM/Tailwind). Replace inner hall div with Konva Stage:

```svelte
<KonvaStage width={stageW} height={stageH} scaleX={viewScale * zoom} scaleY={viewScale * zoom} x={panX} y={panY}>
  <Layer>
    {#each elements as el (el.id)}
      <HallElementNode element={el} {hallWidth} {hallHeight} {dark} mode={mode} />
    {/each}
    {#each tables as t (t.id)}
      <BanquetTable table={t} guests={tableGuests[t.id] ?? []} {hallWidth} {hallHeight} {mode} ... />
    {/each}
  </Layer>
</KonvaStage>
```

`viewScale = min(containerW/hallWidth, containerH/hallHeight)`. Pan/zoom via existing handlers applied to stage `x/y/scale` (remove the old wrapper-div transform). Delete hardcoded stage/aisle/entrance markup and `HALL_LAYOUT` import.

- [ ] **Step 4: Update 3 view pages**

Each page currently does `listTables(...)` and passes `tables` to `HallMap`. Change to load layout:

```ts
const layout = await getPublicLayout(wid); // kiosk
// or getLayout(wid) for authed pages (search, reservation)
tables = layout.tables; elements = layout.elements;
hallWidth = layout.hallWidth; hallHeight = layout.hallHeight;
```

and pass `elements`, `hallWidth`, `hallHeight` to `<HallMap mode="view" ...>`. Kiosk uses `getPublicLayout`; `frontend/src/lib/api/public.ts` `publicListTables` stays for other callers — check each file's actual usage and only swap what feeds HallMap.

- [ ] **Step 5: Verify**

Run: `cd frontend && npm run check`
Expected: PASS for these files (tables/seating pages still broken — Task 7). Then `npm run dev`, open kiosk + search pages: map renders tables + 5 default elements, zoom/pan works, table click shows guests.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/components/seating frontend/src/routes
git commit -m "feat(frontend): konva hall map view mode"
```

---

### Task 7: Edit mode + tables/seating page integration

**Files:**
- Modify: `frontend/src/lib/components/seating/HallMap.svelte`
- Modify: `frontend/src/lib/components/seating/BanquetTable.svelte`
- Modify: `frontend/src/lib/components/seating/HallElementNode.svelte`
- Create: `frontend/src/lib/components/seating/EditToolbar.svelte`
- Modify: `frontend/src/routes/[wid]/tables/+page.svelte`
- Modify: `frontend/src/routes/[wid]/seating/+page.svelte`

**Interfaces:**
- Consumes: `saveLayout`, `defaultSlot`, Task 6 components.
- Produces: edit-mode UX — drag, Transformer (rotate all, resize elements), add-element palette, canvas size inputs, save/cancel.

- [ ] **Step 1: Edit state in HallMap**

In edit mode the map works on local deep copies (`editTables`, `editElements`) so cancel is free:

```ts
let editTables = $state<BanquetTable[]>([]);
let editElements = $state<HallElement[]>([]);
$effect(() => { if (mode === 'edit') { editTables = structuredClone(tables); editElements = structuredClone(elements); } });
```

Selected node id: `let selectedId = $state<string | null>(null)`.

- [ ] **Step 2: Drag + Transformer**

Tables/elements render from edit copies with `draggable`. `on:dragend` writes back `%` position: `t.x = e.target.x() / hallWidth * 100` (clamp 0–100), same for y. Click selects (attach a single Konva `Transformer` to the selected node via `transformer.nodes([node])` — get node refs with `bind:this` into a `Map<string, Konva.Group>`). `on:transformend` writes back degree (and width/height for elements from `node.scaleX()*w`, then reset scale to 1).

Table Transformer config: `enabledAnchors: []` (rotate only). Element Transformer: all anchors.

In edit mode, `onTableClick`/`onSeatClick` handlers are NOT wired (guest view disabled).

- [ ] **Step 3: `EditToolbar.svelte`**

Fixed bar above the map (inside HallMap when `mode==='edit'`):

- Add buttons: Stage, DJ, Entrance, TV, Walkway, Box → push to `editElements`:
  ```ts
  const defaults: Record<ElementType, {w:number;h:number;label:string}> = {
    stage: { w: 55, h: 6, label: 'Stage' },
    dj_counter: { w: 12, h: 5, label: 'DJ' },
    entrance: { w: 14, h: 4, label: 'Entrance' },
    tv: { w: 5, h: 3, label: 'TV' },
    walkway: { w: 3, h: 40, label: '' },
    box: { w: 25, h: 30, label: '' },
  };
  // spawn: { id: '', type, x: 50, y: 50, degree: 0, width: w, height: h, label, zIndex: 10 }
  ```
- Delete-selected button (elements only).
- Canvas inputs: `hallWidth`, `hallHeight` number inputs bound to local edit values.
- Save / Cancel buttons → call props `onSaveLayout(editTables, editElements, hallWidth, hallHeight)` / `onCancelEdit`.

- [ ] **Step 4: Tables page integration**

- Add `editMode` state + "Edit layout" button toggling it; pass `mode={editMode ? 'edit' : 'view'}` plus layout props to HallMap.
- `handleSaveLayout`: `await saveLayout(wid, { hallWidth, hallHeight, tables: editTables.map(...), elements: editElements })`, toast success, reload layout, exit edit mode. On error: toast, stay in edit mode (state retained).
- Create-table flow: remove `formRow`/`formCol` and `rowColToXY`; compute `const pos = defaultSlot(tables)` at `openCreate()`; send `x: pos.x, y: pos.y, degree: 0` in `handleSave`. `previewTable` uses pos directly.
- Load via `getLayout(wid)` (replaces `listTables` for the map; keep `getOccupancy`).

- [ ] **Step 5: Seating page integration**

Same edit toggle + handlers as tables page (view behavior otherwise unchanged: seat clicking for assignment stays in view mode). Load via `getLayout`.

- [ ] **Step 6: Verify**

Run: `cd frontend && npm run check && npx vitest run`
Expected: PASS. Manual: on tables page toggle Edit layout → drag a table, rotate an element, add a TV, resize canvas, Save → reload, layout persists; kiosk reflects it; Cancel discards.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/components/seating frontend/src/routes
git commit -m "feat(frontend): hall map edit mode with drag/rotate/resize and layout save"
```

---

### Task 8: Cleanup + end-to-end verification

**Files:**
- Delete: `frontend/src/lib/constants/index.ts` `HALL_LAYOUT` (keep other constants)
- Modify: any lingering `row`/`col`/`.x` mirrors

- [ ] **Step 1: Sweep**

Run: `cd frontend && grep -rn 'HALL_LAYOUT\|rowColToXY\|\.row\b\|\.col\b' src/ ; cd ../backend && grep -rn 'computeLayout\|yPositions' internal/`
Expected: no hits (fix any stragglers; `models/layout.go` keeps its private `yPositions`).

- [ ] **Step 2: Full verification**

Run: `cd backend && go build ./... && go test ./... && cd ../frontend && npm run check && npx vitest run && npm run build`
Expected: all PASS.

- [ ] **Step 3: Manual E2E against docker DB**

`docker compose up -d`, run backend + frontend, verify: existing wedding renders identical layout (migration), edit → save → kiosk + checkin modal map match.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove legacy hall layout constants and row/col remnants"
```

---

## Self-Review Notes

- Spec coverage: migration ✓ (T1/T2), hall_elements ✓ (T1/T4), per-wedding canvas ✓ (T1/T4/T7), PATCH layout ✓ (T4), Konva view+edit ✓ (T6/T7), default grid ✓ (T5/T7), cleanup ✓ (T8), tests ✓ (T1/T4/T5).
- `svelte-konva` + Svelte 5 compat is the main risk; `--legacy-peer-deps` fallback noted in T5.
