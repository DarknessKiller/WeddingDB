package models

import "github.com/google/uuid"

// DefaultElements returns a sensible default hall layout.
func DefaultElements(weddingID uuid.UUID) []HallElement {
	mk := func(typ string, x, y, w, h float64, name string, z int) HallElement {
		return HallElement{WeddingID: weddingID, Type: typ, X: x, Y: y, Width: w, Height: h, Name: name, ZIndex: z}
	}
	return []HallElement{
		mk("stage", 50, 3, 30, 6, "Stage", 10),
		mk("entrance", 50, 97, 16, 4, "Entrance", 10),
	}
}
