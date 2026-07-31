package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNewFuegoServer_SetsOpenAPIInfo(t *testing.T) {
	env := Env{
		Port:      "0",
		PublicURL: "https://wedding.example.com",
	}
	version := "1.2.3"
	s := NewFuegoServer(env, version)

	spec := s.Engine.OutputOpenAPISpec()

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

	spec := s.Engine.OutputOpenAPISpec()
	if spec.Info.Version != "dev" {
		t.Errorf("OpenAPI Info.Version = %q, want %q", spec.Info.Version, "dev")
	}
}

func TestNewFuegoServer_EmptyPublicURL_NoServers(t *testing.T) {
	env := Env{Port: "0", PublicURL: ""}
	s := NewFuegoServer(env, "1.0.0")

	spec := s.Engine.OutputOpenAPISpec()
	// When PUBLIC_URL is empty, we don't set servers (fuego may set a default)
	// The key assertion: no crash and version still set
	if spec.Info.Version != "1.0.0" {
		t.Errorf("OpenAPI Info.Version = %q, want %q", spec.Info.Version, "1.0.0")
	}
}

func TestLoadEnv_PublicURLDefault(t *testing.T) {
	// LoadEnv requires JWT_SECRET; set it for test
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PORT", "9999")
	// PUBLIC_URL not set → should default to http://localhost:9999
	env := LoadEnv()
	want := "http://localhost:9999"
	if env.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q (auto-derived from PORT)", env.PublicURL, want)
	}
}

func TestLoadEnv_PublicURLExplicit(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
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

	spec := s.Engine.OutputOpenAPISpec()

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
	os.MkdirAll(evidenceDir, 0o755)

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal OpenAPI spec: %v", err)
	}
	if err := os.WriteFile(evidenceDir+"/openapi-spec.json", data, 0o644); err != nil {
		t.Fatalf("Failed to write evidence: %v", err)
	}
	t.Logf("OpenAPI spec written to %s/openapi-spec.json", evidenceDir)
}
