package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-fuego/fuego"
)

func TestNewFuegoServer_SetsOpenAPIInfo(t *testing.T) {
	env := Env{
		Port:      "0",
		PublicURL: "https://wedding.example.com",
	}
	version := "1.2.3"
	s := NewFuegoServer(env, version)

	spec := s.OutputOpenAPISpec()

	// Version must be injected from build arg
	if spec.Info.Version != version {
		t.Errorf("OpenAPI Info.Version = %q, want %q", spec.Info.Version, version)
	}
	if spec.Info.Title != "WeddingDB API" {
		t.Errorf("OpenAPI Info.Title = %q, want %q", spec.Info.Title, "WeddingDB API")
	}
	if spec.Info.Description != "Wedding guest management and seating API" {
		t.Errorf("OpenAPI Info.Description = %q", spec.Info.Description)
	}

	// Server URL must be set from PUBLIC_URL
	if len(spec.Servers) == 0 {
		t.Fatal("OpenAPI Servers is empty")
	}
	if spec.Servers[0].URL != "https://wedding.example.com" {
		t.Errorf("OpenAPI Servers[0].URL = %q, want %q", spec.Servers[0].URL, "https://wedding.example.com")
	}
}

func TestNewFuegoServer_DefaultVersion(t *testing.T) {
	env := Env{Port: "0"}
	s := NewFuegoServer(env, "dev")

	spec := s.OutputOpenAPISpec()
	if spec.Info.Version != "dev" {
		t.Errorf("OpenAPI Info.Version = %q, want %q", spec.Info.Version, "dev")
	}
}

func TestNewFuegoServer_EmptyPublicURL_NoServers(t *testing.T) {
	env := Env{Port: "0", PublicURL: ""}
	s := NewFuegoServer(env, "1.0.0")

	spec := s.OutputOpenAPISpec()
	// When PUBLIC_URL is empty, we don't set servers (fuego may set a default)
	// The key assertion: no crash and version still set
	if spec.Info.Version != "1.0.0" {
		t.Errorf("OpenAPI Info.Version = %q, want %q", spec.Info.Version, "1.0.0")
	}
}

func TestLoadEnv_PublicURLDefault(t *testing.T) {
	// LoadEnv requires JWT_SECRET; set it for test
	t.Setenv("JWT_SECRET", "test-secret-0123456789abcdef0123456789")
	t.Setenv("PORT", "9999")
	// PUBLIC_URL not set → should default to http://localhost:9999
	env := LoadEnv()
	want := "http://localhost:9999"
	if env.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q (auto-derived from PORT)", env.PublicURL, want)
	}
}

func TestLoadEnv_PublicURLExplicit(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-0123456789abcdef0123456789")
	t.Setenv("PUBLIC_URL", "https://prod.example.com")
	env := LoadEnv()
	if env.PublicURL != "https://prod.example.com" {
		t.Errorf("PublicURL = %q, want %q", env.PublicURL, "https://prod.example.com")
	}
}

func TestOpenAPI_SpecJSON(t *testing.T) {
	env := Env{
		Port:      "0",
		PublicURL: "https://wedding.example.com",
	}
	s := NewFuegoServer(env, "2.0.0")

	spec := s.OutputOpenAPISpec()

	// Verify key fields
	if spec.Info.Title != "WeddingDB API" {
		t.Errorf("Title = %q", spec.Info.Title)
	}
	if spec.Info.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", spec.Info.Version, "2.0.0")
	}
	if len(spec.Servers) == 0 || spec.Servers[0].URL != "https://wedding.example.com" {
		t.Errorf("Server URL = %v", spec.Servers)
	}

	// Write spec to evidence directory
	evidenceDir := "/tmp/no-mistakes-evidence/01KYWB93P3SW3JGSQQA2NPGTFZ"
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("Failed to create evidence dir: %v", err)
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal OpenAPI spec: %v", err)
	}
	if err := os.WriteFile(evidenceDir+"/openapi-spec.json", data, 0o644); err != nil {
		t.Fatalf("Failed to write evidence: %v", err)
	}
	t.Logf("OpenAPI spec written to %s/openapi-spec.json", evidenceDir)
}

func TestNewFuegoServer_TimeoutsForLongTransfers(t *testing.T) {
	// fuego defaults Read/WriteTimeout to 30s; both would reset large uploads
	// and SSE streams mid-transfer. Zeroing them is the app-level contract
	// behind "the reverse proxy owns request limits".
	s := NewFuegoServer(Env{Port: "0"}, "test")
	if s.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 (proxy owns transfer duration)", s.ReadTimeout)
	}
	if s.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (SSE must not be cut)", s.WriteTimeout)
	}
	if s.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout = 0, want non-zero (slowloris guard)")
	}
}

func TestInternalErrorHandler_PassesFuegoErrorsThrough(t *testing.T) {
	err := internalErrorHandler(context.Background(), fuego.BadRequestError{Title: "bad input"})
	var httpErr fuego.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v, want HTTPError", err)
	}
	if httpErr.StatusCode() != 400 {
		t.Errorf("status = %d, want 400", httpErr.StatusCode())
	}
}

func TestInternalErrorHandler_HidesUnexpectedErrors(t *testing.T) {
	err := internalErrorHandler(context.Background(), errors.New("pq: relation secret_table does not exist"))
	var httpErr fuego.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v, want HTTPError", err)
	}
	if httpErr.StatusCode() != 500 {
		t.Errorf("status = %d, want 500", httpErr.StatusCode())
	}
	if strings.Contains(httpErr.Error(), "secret_table") {
		t.Fatalf("response leaks the internal error: %v", httpErr)
	}
}

// leakyError mimics driver errors such as *pgconn.PgError, whose exported
// fields get serialized by the default error writer.
type leakyError struct{ Message string }

func (e leakyError) Error() string { return e.Message }

func TestNewFuegoServer_UnexpectedErrorBecomesGeneric500(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"}, "test")
	fuego.Get(s, "/boom", func(fuego.ContextNoBody) (any, error) {
		return nil, leakyError{Message: "pq: relation secret_table does not exist"}
	})

	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret_table") {
		t.Fatalf("response leaks the internal error: %s", rec.Body.String())
	}
}

func TestNewFuegoServer_RejectsOversizedBody(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"}, "test")
	fuego.Post(s, "/echo", func(c fuego.ContextWithBody[map[string]string]) (any, error) {
		if _, err := c.Body(); err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid request"}
		}
		return map[string]string{"ok": "1"}, nil
	})

	payload := `{"x":"` + strings.Repeat("a", 2*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a body over the limit", rec.Code)
	}
}
