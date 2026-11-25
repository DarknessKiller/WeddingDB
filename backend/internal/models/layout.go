package models

import "github.com/google/uuid"

// ReconcileElements diffs the desired hall elements against the stored ones
// (full-replace semantics): an incoming element without an ID is a create, one
// with an ID is an update, and a stored element absent from the payload is a
// delete. The layout save runs it inside its transaction, so two concurrent
// saves cannot interleave their read and write.
func ReconcileElements(existing, incoming []HallElement) (toCreate, toUpdate []HallElement, toDelete []uuid.UUID) {
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

// DefaultElements returns a sensible default hall layout.
func DefaultElements(weddingID uuid.UUID) []HallElement {
	mk := func(typ string, x, y, w, h float64, name string, z int) HallElement {
		return HallElement{ID: uuid.New(), WeddingID: weddingID, Type: typ, X: x, Y: y, Width: w, Height: h, Name: name, ZIndex: z}
	}
	return []HallElement{
		mk("stage", 50, 3, 30, 6, "Stage", 10),
		mk("entrance", 50, 97, 16, 4, "Entrance", 10),
	}
}
