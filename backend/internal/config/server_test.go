package config

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The API surface must not be discoverable: an anonymous caller should not be
// able to enumerate routes or read schemas from a generated spec.
func TestNewFuegoServer_OpenAPINotPublished(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"})

	for _, path := range []string{"/swagger", "/swagger/", "/swagger/index.html", "/swagger/openapi.json", "/openapi.json"} {
		w := httptest.NewRecorder()
		s.Mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
	}
}

func TestNewFuegoServer_TimeoutsForLongTransfers(t *testing.T) {
	// fuego defaults Read/WriteTimeout to 30s; both would reset large uploads
	// and SSE streams mid-transfer. Zeroing them is the app-level contract
	// behind "the reverse proxy owns request limits".
	s := NewFuegoServer(Env{Port: "0"})
	if s.Server.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 (proxy owns transfer duration)", s.Server.ReadTimeout)
	}
	if s.Server.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (SSE must not be cut)", s.Server.WriteTimeout)
	}
	if s.Server.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout = 0, want non-zero (slowloris guard)")
	}
}
