package tracing

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetupDisabledByDefault(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "")
	shutdown, ok := Setup("test")
	if ok || shutdown != nil {
		t.Fatalf("Setup with OTEL_ENABLED unset: ok=%v shutdownSet=%v, want false nil", ok, shutdown != nil)
	}
}

func TestMiddlewareDoesNotBreakHandler(t *testing.T) {
	// Register a no-op provider + propagator so middleware spans are local-only.
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		otel.SetTracerProvider(nil)
		otel.SetTextMapPropagator(nil)
	}()

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/weddings/{wid}/guests", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGormPluginNoRowsNoPanic(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	defer func() { otel.SetTracerProvider(nil) }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Use(NewGormPlugin()); err != nil {
		t.Fatalf("register plugin: %v", err)
	}

	type item struct {
		ID   uint `gorm:"primaryKey"`
		Name string
	}
	if err := db.AutoMigrate(&item{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// First query: no rows. Must not panic; ErrRecordNotFound is expected.
	var missing item
	if err := db.First(&missing, 999).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("First(nonexistent) error = %v, want ErrRecordNotFound", err)
	}

	// Second query: existing row must still work after the no-rows path.
	if err := db.Create(&item{Name: "existing"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var found item
	if err := db.First(&found, 1).Error; err != nil {
		t.Fatalf("First(existing) error = %v, want row", err)
	}
	if found.Name != "existing" {
		t.Fatalf("First(existing) name = %q, want %q", found.Name, "existing")
	}
}

func TestSpanNameFormatter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/weddings/abc/guests", nil)
	if got := spanName("", req); got != "GET /api/weddings/abc/guests" {
		t.Fatalf("spanName fallback = %q", got)
	}
	req.Pattern = "GET /api/weddings/{wid}/guests"
	if got := spanName("", req); got != "GET /api/weddings/{wid}/guests" {
		t.Fatalf("spanName pattern = %q", got)
	}
}
