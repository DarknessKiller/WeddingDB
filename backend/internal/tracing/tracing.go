// Package tracing provides opt-in OpenTelemetry tracing: an OTLP exporter,
// an HTTP server middleware, and a GORM query-span plugin. Disabled unless
// OTEL_ENABLED=true.
package tracing

import (
	"context"
	"log"
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const tracerName = "weddingdb/tracing"

// Setup initializes global OpenTelemetry tracing. Opt-in: returns
// (nil, false) unless OTEL_ENABLED=true. The returned shutdown flushes
// pending spans and must be called on graceful exit.
func Setup(version string) (func(context.Context) error, bool) {
	if os.Getenv("OTEL_ENABLED") != "true" {
		return nil, false
	}

	ctx := context.Background()

	// Drop-on-failure: no retries, so a down collector can't balloon memory.
	// Default endpoint is localhost:4318/v1/traces; overridable via the
	// standard OTEL_EXPORTER_OTLP_ENDPOINT env var (path optional).
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318/v1/traces")),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
	)
	if err != nil {
		log.Println("Warning: tracing disabled, OTLP exporter setup failed:", err)
		return nil, false
	}

	// Parent-based: head sampling decisions come from the upstream
	// traceparent; root spans always sample so Jaeger UI stays useful.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("weddingdb"),
			semconv.ServiceVersion(version),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, true
}

// Middleware wraps next with an otelhttp span-per-request handler. Span name
// uses the ServeMux route pattern when known (e.g. "GET /api/weddings/{wid}/guests"),
// falling back to "METHOD path".
func Middleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "weddingdb",
		otelhttp.WithSpanNameFormatter(spanName),
	)
}

func spanName(_ string, r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

// GormPlugin creates a "gorm.query" span per DB query. Statement text is only
// recorded when OTEL_LOG_SQL=all.
type GormPlugin struct {
	tracer trace.Tracer
	logSQL bool
}

func NewGormPlugin() *GormPlugin {
	return &GormPlugin{
		tracer: otel.Tracer(tracerName),
		logSQL: os.Getenv("OTEL_LOG_SQL") == "all",
	}
}

// Name implements gorm.Plugin.
func (p *GormPlugin) Name() string { return "tracing" }

// Initialize implements gorm.Plugin; hooks into query execution.
func (p *GormPlugin) Initialize(db *gorm.DB) error {
	_ = db.Callback().Query().Before("gorm:query").Register("tracing:before_query", p.before)
	_ = db.Callback().Create().Before("gorm:create").Register("tracing:before_create", p.before)
	_ = db.Callback().Update().Before("gorm:update").Register("tracing:before_update", p.before)
	_ = db.Callback().Delete().Before("gorm:delete").Register("tracing:before_delete", p.before)
	_ = db.Callback().Query().After("gorm:query").Register("tracing:after_query", p.after)
	_ = db.Callback().Create().After("gorm:create").Register("tracing:after_create", p.after)
	_ = db.Callback().Update().After("gorm:update").Register("tracing:after_update", p.after)
	_ = db.Callback().Delete().After("gorm:delete").Register("tracing:after_delete", p.after)
	return nil
}

func (p *GormPlugin) before(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Context == nil {
		return
	}
	attrs := []attribute.KeyValue{
		semconv.DBSystemNamePostgreSQL,
	}
	if p.logSQL {
		attrs = append(attrs, semconv.DBQueryText(db.Statement.SQL.String()))
	}
	ctx, span := p.tracer.Start(db.Statement.Context, "gorm.query", trace.WithAttributes(attrs...))
	db.Statement.Context = ctx
	db.InstanceSet("tracing:span", span)
}

func (p *GormPlugin) after(db *gorm.DB) {
	v, ok := db.InstanceGet("tracing:span")
	if !ok {
		return
	}
	span, ok := v.(trace.Span)
	if !ok || span == nil {
		return
	}
	defer span.End()
	span.SetAttributes(semconv.DBResponseReturnedRows(int(db.Statement.RowsAffected)))
	if err := db.Statement.Error; err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
