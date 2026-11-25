package models

import (
	"testing"

	"github.com/google/uuid"
)

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

func TestReconcileElements(t *testing.T) {
	wid := uuid.New()
	keep := HallElement{ID: uuid.New(), WeddingID: wid, Type: "stage"}
	drop := HallElement{ID: uuid.New(), WeddingID: wid, Type: "tv"}
	existing := []HallElement{keep, drop}

	updated := keep
	updated.X = 42
	fresh := HallElement{WeddingID: wid, Type: "box", X: 10, Y: 10, Width: 20, Height: 20}
	incoming := []HallElement{updated, fresh}

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
