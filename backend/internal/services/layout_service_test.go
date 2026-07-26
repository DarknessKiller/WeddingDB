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
