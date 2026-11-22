package handlers

import (
	"net/http"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

func TestTableCreate_PositionBounds(t *testing.T) {
	db := newGuestTestDB(t)
	wid := uuid.New()
	svc := services.NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/tables", NewTableHandler(svc).Create)
	path := "/api/weddings/" + wid.String() + "/tables"

	tests := []struct {
		name, body string
		want       int
	}{
		{"top right corner", `{"name":"A","capacity":4,"x":100,"y":100}`, http.StatusCreated},
		{"x out of range", `{"name":"B","capacity":4,"x":101,"y":50}`, http.StatusBadRequest},
		{"y out of range", `{"name":"C","capacity":4,"x":50,"y":-1}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveJSON(s, http.MethodPost, path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
