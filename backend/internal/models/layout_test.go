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
