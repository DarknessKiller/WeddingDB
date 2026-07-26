package models

import "github.com/google/uuid"

var yPositions = map[int]float64{1: 15, 2: 30, 3: 45, 4: 60, 5: 75, 6: 90}

// RowColToXY computes x/y percentages from legacy row/col values.
// rows[i], cols[i] pair per table; ids[i] identifies the table.
func RowColToXY(ids []uuid.UUID, rows, cols []int) map[uuid.UUID][2]float64 {
	maxRow, maxCol := 0, 0
	for _, r := range rows {
		if r > maxRow {
			maxRow = r
		}
	}
	for _, c := range cols {
		if c > maxCol {
			maxCol = c
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
	out := make(map[uuid.UUID][2]float64, len(ids))
	for i, id := range ids {
		y, ok := yPos[rows[i]]
		if !ok || y == 0 {
			y = 50
		}
		out[id] = [2]float64{100.0 / float64(maxCol+1) * float64(cols[i]), y}
	}
	return out
}

// DefaultElements returns the hardcoded hall decoration elements.
func DefaultElements(weddingID uuid.UUID) []HallElement {
	mk := func(typ string, x, y, w, h float64, label string, z int) HallElement {
		return HallElement{WeddingID: weddingID, Type: typ, X: x, Y: y, Width: w, Height: h, Label: label, ZIndex: z}
	}
	return []HallElement{
		mk("stage", 50, 3, 55, 6, "\u2726 Stage \u2726", 10),
		mk("entrance", 50, 98, 14, 4, "\u25bc Entrance \u25bc", 10),
		mk("walkway", 50, 50, 0.3, 84, "", 1),
		mk("walkway", 30, 50, 0.3, 84, "", 1),
		mk("walkway", 70, 50, 0.3, 84, "", 1),
	}
}
