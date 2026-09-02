package tracing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestGormPluginRecordsSQLWithEnv(t *testing.T) {
	exp := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp)))
	defer otel.SetTracerProvider(nil)

	t.Setenv("OTEL_LOG_SQL", "all")
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

	ctx, span := otel.Tracer("test").Start(context.Background(), "root")
	db = db.WithContext(ctx)
	if err := db.Create(&item{Name: "x"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got item
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	span.End()
	time.Sleep(50 * time.Millisecond)

	sawSQL := false
	for _, s := range exp.Ended() {
		if s.Name() != "gorm.query" {
			continue
		}
		for _, e := range s.Events() {
			for _, kv := range e.Attributes {
				if string(kv.Key) == "db.query.text" && kv.Value.AsString() != "" {
					sawSQL = true
				}
			}
		}
	}
	if !sawSQL {
		t.Fatalf("no gorm.query span with non-empty db.query.text; spans: %v", exp.Ended())
	}
}

func TestGormPluginNoSQLWithoutEnv(t *testing.T) {
	exp := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp)))
	defer otel.SetTracerProvider(nil)

	t.Setenv("OTEL_LOG_SQL", "")
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

	ctx, span := otel.Tracer("test").Start(context.Background(), "root")
	db = db.WithContext(ctx)
	if err := db.Create(&item{Name: "x"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got item
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	span.End()
	time.Sleep(50 * time.Millisecond)

	for _, s := range exp.Ended() {
		if s.Name() != "gorm.query" {
			continue
		}
		for _, e := range s.Events() {
			for _, kv := range e.Attributes {
				if string(kv.Key) == "db.query.text" {
					t.Fatalf("gorm.query span has db.query.text without OTEL_LOG_SQL=all: %v", e.Attributes)
				}
			}
		}
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

func TestMiddlewareSpanNameAndBodies(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() { otel.SetTracerProvider(nil); otel.SetTextMapPropagator(nil) }()

	t.Setenv("OTEL_LOG_BODY", "all")
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Body must be readable after middleware capture.
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"user":"x"}` {
			t.Errorf("downstream handler got mangled body: %q", string(b))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"user":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("response body = %q", rec.Body.String())
	}
}

func TestBodyAttrMasksPassword(t *testing.T) {
	in := []byte(`{"email":"x@y.z","password":"hunter2"}`)
	got := maskPassword(in)
	if strings.Contains(got, "hunter2") {
		t.Fatalf("password leaked into span body: %s", got)
	}
	if !strings.Contains(got, `"password":"***"`) {
		t.Fatalf("password not masked: %s", got)
	}
	if !strings.Contains(got, "x@y.z") {
		t.Fatalf("other fields lost: %s", got)
	}

	// Case-insensitive key, nested bodies, non-JSON: pass-through untouched.
	if got := maskPassword([]byte(`{"PASSWORD":"p"}`)); strings.Contains(got, "p") {
		t.Fatalf("PASSWORD not masked: %s", got)
	}
	if got := maskPassword([]byte(`not json`)); got != "not json" {
		t.Fatalf("non-JSON body mangled: %s", got)
	}
	if got := maskPassword([]byte(`{"name":"x"}`)); got != `{"name":"x"}` {
		t.Fatalf("JSON without password mangled: %s", got)
	}
}

func TestMiddlewareCapturesPattern(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	defer func() { otel.SetTracerProvider(nil) }()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/weddings/{wid}/guests", func(w http.ResponseWriter, r *http.Request) {
		if r.Pattern != "POST /api/weddings/{wid}/guests" {
			t.Errorf("pattern inside handler = %q", r.Pattern)
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/abc/guests", nil)
	mux.ServeHTTP(httptest.NewRecorder(), req)
}
