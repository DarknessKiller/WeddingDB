package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/config"
)

func TestRegisterRoutesJSONBodyLimit(t *testing.T) {
	s := config.NewFuegoServer(config.Env{Port: "0"}, "test")
	RegisterRoutes(s, nil, nil, nil, nil, nil, nil, nil)
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"limit", `{}` + strings.Repeat(" ", 1048574), http.StatusBadRequest},
		{"over limit", `{}` + strings.Repeat(" ", 1048575), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.ContentLength = -1
			w := httptest.NewRecorder()
			s.Mux.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
