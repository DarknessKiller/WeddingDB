# Hall Map Editor — Design

Date: 2026-07-26

## Goal

Replace the fixed row/col banquet layout with a fully customizable, per-wedding hall map: tables and hall elements (stage, DJ counter, entrance, TV, walkway, generic box) are freely placed, rotated, and resized on a canvas via a drag-and-drop editor, so users can reproduce their venue's real floor plan.

Consumers of the map: tables page, seating page, kiosk screen, check-in/search map. Positioning is owned entirely by the frontend; the backend stores and returns coordinates verbatim.

## Decisions (from brainstorming)

| Question | Decision |
|---|---|
| Rendering | Rewrite `HallMap.svelte` on `svelte-konva` (Konva canvas), one shared component with `mode: 'view' \| 'edit'` |
| Element storage | New `hall_elements` table |
| Rotation | Both tables and elements carry `degree` |
| Existing data | Migration converts row/col → x/y and seeds default elements |
| New table placement | Frontend computes next default grid slot (auto-grid, then draggable) |
| Edit surfaces | Tables page AND seating page get edit mode; kiosk/checkin stay view-only |
| Canvas size | Per-wedding `hall_width`/`hall_height`, editable in edit mode |

## Data Model

### `banquet_tables` (migration)

- Add `x REAL NOT NULL DEFAULT 0`, `y REAL NOT NULL DEFAULT 0`, `degree REAL NOT NULL DEFAULT 0` — percentages of hall canvas (0–100).
- Backfill x/y from existing row/col using the exact `computeLayout` math from `backend/internal/handlers/table.go` (yPositions map + `100/(maxCol+1)*col`), computed **per wedding**.
- Drop `row`, `col` columns.
- Delete `computeLayout` and `yPositions` from the backend; `TableResponse` returns stored x/y/degree verbatim.

### New `hall_elements`

```
id         uuid PK
wedding_id uuid NOT NULL (index, FK)
type       text NOT NULL  -- stage | dj_counter | entrance | tv | walkway | box
x, y       real NOT NULL  -- % of canvas
degree     real NOT NULL DEFAULT 0
width      real NOT NULL  -- % of canvas width
height     real NOT NULL  -- % of canvas height
label      text           -- free text, e.g. "舞台 STAGE", "VIP Room"
z_index    int NOT NULL DEFAULT 0
created_at, updated_at
```

Every element is a rect; `type` drives default styling (walkway = dark fill, box = outline only, tv = small dark rect, entrance = bar, stage/dj_counter = filled rect with label). Multiples allowed (2 TVs, L-shaped walkway = 2 rotated walkway rects, right-side rooms = `box`).

### `weddings`

- Add `hall_width INT NOT NULL DEFAULT 860`, `hall_height INT NOT NULL DEFAULT 1000` (virtual canvas units).

### Default elements (seeded by migration for existing weddings, and on wedding creation)

Matches the current hardcoded `HallMap` decorations so existing weddings render unchanged:

| type | x | y | width | height | label |
|---|---|---|---|---|---|
| stage | 50 | 3 | 55 | 6 | ✦ Stage ✦ |
| entrance | 50 | 98 | 14 | 4 | ▼ Entrance ▼ |
| walkway (main aisle) | 50 | 50 | 0.3 | 84 | — |
| walkway (side) | 30 | 50 | 0.3 | 84 | — |
| walkway (side) | 70 | 50 | 0.3 | 84 | — |

(x/y are element centers, consistent with table x/y semantics.)

## API

- `GET /api/weddings/{wid}/layout` → `{ tables: [{id, name, capacity, x, y, degree, isVip}], elements: [...], hallWidth, hallHeight }`
- Public variant `GET /api/public/weddings/{wid}/layout` for kiosk/reservation (same shape, no auth — mirrors existing public tables endpoint).
- `PATCH /api/weddings/{wid}/layout` — single atomic save from edit mode:
  ```json
  {
    "hallWidth": 1200,
    "hallHeight": 1600,
    "tables": [{ "id": "...", "x": 25, "y": 30, "degree": 0 }],
    "elements": [{ "id": "...", "type": "stage", "x": 50, "y": 3, "degree": 0, "width": 55, "height": 6, "label": "Stage", "zIndex": 0 }]
  }
  ```
  - Tables: position/degree-only updates by id; never created or deleted here.
  - Elements: full-replace semantics — upsert by id, create when id absent, delete any existing element not present in the list. Whole body applied in one transaction.
- Existing table CRUD endpoints stay; create/update accept `x`, `y`, `degree` (frontend always supplies them — auto-grid slot for creates). `row`/`col` removed from `TableRequest`.

## Frontend

### `HallMap.svelte` — Konva rewrite

One component (`svelte-konva`), props: `mode: 'view' | 'edit'`, `tables`, `elements`, `hallWidth`, `hallHeight`, `tableGuests`, selection/highlight props, `onTableClick`, `onSeatClick`, `onSaveLayout`.

- Virtual stage sized `hallWidth × hallHeight`; stored % positions resolved against it. Existing zoom/pan behavior preserved.
- **View mode** (kiosk, search/checkin modal, reservation): no drag, no Transformer; table click → guest view, seat click → as today.
- **Edit mode** (tables page + seating page, behind "Edit layout" toggle):
  - Tables and elements `draggable`.
  - Click selects a node → Konva `Transformer` attached: rotate handle for tables and elements; resize handles for elements only (table radius derives from capacity).
  - Table click-to-view-guests and seat clicks disabled in edit mode.
  - Element palette: Add stage / DJ counter / entrance / TV / walkway / box — spawns a default-sized rect at canvas center.
  - Canvas size inputs (width/height) in the edit toolbar.
  - Save → one `PATCH /layout` with all positions + element list; Cancel → reload from server, discarding local edits.
  - Failed save → error toast, local edit state retained (no silent revert).

### `BanquetTable.svelte` → Konva group

SVG becomes a Konva `Group`: table circle, occupancy ring (`Arc`), name/pax `Text`, seat orbit circles rotated by `degree` (seat 1 orientation is meaningful). Seats remain clickable in view mode. Visual styling unchanged.

### Default grid util

Port `yPositions` + column math to `frontend/src/lib/utils/layout.ts` as `defaultSlot(existingTables): { x, y }`; used when creating a table so un-dragged tables produce today's grid look.

### Cleanup

- Hardcoded stage/aisle/entrance markup removed from `HallMap.svelte` (now element data).
- `HALL_LAYOUT` constant removed; canvas dims come from the wedding.
- Frontend `BanquetTable` type: `row`/`col` → `x`, `y`, `degree`.
- Tables page: row/col form fields removed from create/edit modal (placement is on canvas); `rowColToXY` mirror deleted.

## Error Handling

- `PATCH /layout`: validate ids belong to the wedding; whole payload in one DB transaction; 400 on unknown table id.
- Layout load failure on any surface: existing error UI + retry pattern.
- No silent ignore of save errors; local edit state preserved on failure.

## Testing

- Backend: table-driven tests for the migration backfill math (row/col → x/y equivalence with old `computeLayout`) and for `PATCH /layout` element reconcile (upsert / create / delete / transaction rollback on bad id).
- Frontend: unit test for `defaultSlot` util.
