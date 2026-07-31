package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-fuego/fuego"
)

// TestSetStatus_Created verifies fuego handlers can return 201 via SetStatus.
// This is the integration point the PR relies on for all create endpoints.
func TestSetStatus_Created(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())

	fuego.Post(s, "/test-create", func(c fuego.ContextWithBody[any]) (any, error) {
		c.SetStatus(http.StatusCreated)
		return map[string]string{"id": "123"}, nil
	})

	req := httptest.NewRequest("POST", "/test-create", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("SetStatus(201) → status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// TestSetStatus_NoContent verifies fuego handlers can return 204 via SetStatus.
// This is the integration point the PR relies on for all delete/void endpoints.
func TestSetStatus_NoContent(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())

	fuego.Delete(s, "/test-delete/{id}", func(c fuego.ContextNoBody) (any, error) {
		c.SetStatus(http.StatusNoContent)
		return nil, nil
	})

	req := httptest.NewRequest("DELETE", "/test-delete/abc", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("SetStatus(204) → status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestSetStatus_DefaultIsOK verifies the default status is 200 when SetStatus is not called.
func TestSetStatus_DefaultIsOK(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())

	fuego.Get(s, "/test-default", func(c fuego.ContextNoBody) (any, error) {
		return map[string]string{"ok": "true"}, nil
	})

	req := httptest.NewRequest("GET", "/test-default", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("default status = %d, want %d", rec.Code, http.StatusOK)
	}
}
