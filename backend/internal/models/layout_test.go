package models

import (
	"testing"

	"github.com/google/uuid"
)

func TestRowColToXY_FixedYPositions(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	rows := []int{1, 1, 3}
	cols := []int{1, 2, 1}
	got := RowColToXY(ids, rows, cols)
	// yPositions: row1=15, row3=45; maxCol=2 -> x = 100/3*col
	if got[ids[0]] != [2]float64{100.0 / 3.0, 15} {
		t.Errorf("row1col1: got %v", got[ids[0]])
	}
	if got[ids[1]] != [2]float64{100.0 / 3.0 * 2, 15} {
		t.Errorf("row1col2: got %v", got[ids[1]])
	}
	if got[ids[2]] != [2]float64{100.0 / 3.0, 45} {
		t.Errorf("row3col1: got %v", got[ids[2]])
	}
}

func TestRowColToXY_BeyondSixRows(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	rows := []int{1, 7}
	cols := []int{1, 1}
	got := RowColToXY(ids, rows, cols)
	// maxRow=7 > 6 -> linear spread 12..88: row1=12, row7=88
	if got[ids[0]][1] != 12 {
		t.Errorf("row1 y: got %v", got[ids[0]][1])
	}
	if got[ids[1]][1] != 88 {
		t.Errorf("row7 y: got %v", got[ids[1]][1])
	}
}

func TestDefaultElements(t *testing.T) {
	wid := uuid.New()
	els := DefaultElements(wid)
	if len(els) != 2 {
		t.Fatalf("want 2 default elements, got %d", len(els))
	}
	counts := map[string]int{}
	for _, e := range els {
		if e.WeddingID != wid {
			t.Errorf("wrong wedding id")
		}
		counts[e.Type]++
	}
	if counts["stage"] != 1 || counts["entrance"] != 1 {
		t.Errorf("bad defaults: %v", counts)
	}
}
