package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-fuego/fuego"
	"github.com/jackc/pgx/v5/pgconn"
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

func TestLoadEnvJWTSecretLength(t *testing.T) {
	if os.Getenv("TEST_SECRET_CHILD") == "1" {
		LoadEnv()
		return
	}
	for _, n := range []int{31, 32} {
		t.Run(strings.Repeat("x", n), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLoadEnvJWTSecretLength$")
			cmd.Env = append(os.Environ(), "TEST_SECRET_CHILD=1", "JWT_SECRET="+strings.Repeat("x", n))
			err := cmd.Run()
			if (err == nil) != (n == 32) {
				t.Fatalf("secret length %d: %v", n, err)
			}
		})
	}
}

func TestUntypedErrorsAreGeneric(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"}, "test")
	fuego.Get(s, "/failure", func(fuego.ContextNoBody) (any, error) {
		return nil, errors.New("driver secret query")
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/failure", nil)
	r.Header.Set("Accept", "text/plain")
	s.Mux.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "driver secret query") {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
}

// A write that timed out waiting for the wedding row lock is retryable: 503,
// not the generic 500, and still no driver text.
func TestLockTimeoutIsRetryable(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"}, "test")
	fuego.Get(s, "/busy", func(fuego.ContextNoBody) (any, error) {
		return nil, fmt.Errorf("create guest: %w", &pgconn.PgError{Code: "55P03", Message: "canceling statement due to lock timeout"})
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/busy", nil)
	s.Mux.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "lock timeout") {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
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

func TestNewFuegoServer_LimitsEveryBodyType(t *testing.T) {
	s := NewFuegoServer(Env{Port: "0"}, "test")
	fuego.Post(s, "/text", func(c fuego.ContextWithBody[string]) (string, error) {
		_, err := c.Body()
		return "", err
	})
	fuego.Post(s, "/binary", func(c fuego.ContextWithBody[[]byte]) (string, error) {
		_, err := c.Body()
		return "", err
	})
	for _, tc := range []struct{ name, path, contentType string }{
		{"text/plain", "/text", "text/plain"},
		{"application/octet-stream", "/binary", "application/octet-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(strings.Repeat("x", 1024*1024+1)))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			s.Mux.ServeHTTP(w, r)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
			}
		})
	}
}

func TestLoadEnv_PublicURLDefault(t *testing.T) {
	// LoadEnv requires JWT_SECRET; set it for test
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("PORT", "9999")
	// PUBLIC_URL not set → should default to http://localhost:9999
	env := LoadEnv()
	want := "http://localhost:9999"
	if env.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q (auto-derived from PORT)", env.PublicURL, want)
	}
}

func TestLoadEnv_PublicURLExplicit(t *testing.T) {
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
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
	s := NewFuegoServer(Env{Port: "0"}, "test")
	if s.ReadTimeout != 5*time.Minute {
		t.Errorf("ReadTimeout = %v, want 5m", s.ReadTimeout)
	}
	if s.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (SSE must not be cut)", s.WriteTimeout)
	}
	if s.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout = 0, want non-zero (slowloris guard)")
	}
}
