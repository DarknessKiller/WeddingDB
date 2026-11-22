package bootstrap

import (
	"strings"
	"testing"

	"weddingdb/internal/config"
)

func TestResolveDatabaseURL_UsesEnvURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	got, err := resolveDatabaseURL(config.Env{DatabaseURL: "postgresql://u:p@db/app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "postgresql://u:p@db/app" {
		t.Fatalf("got %q, want the DATABASE_URL value", got)
	}
}

func TestResolveDatabaseURL_RequiresCredentials(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
		t.Setenv(key, "")
	}

	_, err := resolveDatabaseURL(config.Env{})
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL and DB_* credentials are unset")
	}
	for _, key := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

func TestResolveDatabaseURL_BuildsFromCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USER", "wedding")
	t.Setenv("DB_PASSWORD", "p@ss word")
	t.Setenv("DB_NAME", "weddingdb")
	t.Setenv("DB_SSLMODE", "require")

	got, err := resolveDatabaseURL(config.Env{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "postgresql://wedding:p%40ss+word@db.internal:6543/weddingdb?sslmode=require"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCheckBootstrapPassword_RejectsWeakValues(t *testing.T) {
	for _, password := range []string{"", "password", "replace-with-a-strong-one-time-password", "NoDigits!"} {
		if err := checkBootstrapPassword(password); err == nil {
			t.Errorf("%q accepted, want rejected", password)
		}
	}
	if err := checkBootstrapPassword("GoodPass1!"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
}
