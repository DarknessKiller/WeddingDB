// Package tracing provides opt-in OpenTelemetry tracing: an OTLP exporter,
// an HTTP server middleware, and a GORM query-span plugin. Disabled unless
// OTEL_ENABLED=true.
package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

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

// maxBodyLog caps captured request/response bodies; larger payloads record a
// truncation marker instead.
const maxBodyLog = 8 * 1024

// addBodyEvent records a request/response body as a span event (shows in
// Jaeger's span detail logs, not as a tag).
func addBodyEvent(span trace.Span, name string, body []byte) {
	if len(body) > maxBodyLog {
		body = append(body[:maxBodyLog:maxBodyLog], []byte("...[truncated]")...)
	}
	span.AddEvent(name, trace.WithAttributes(attribute.String("body", maskPassword(body))))
}

// maskPassword replaces the top-level JSON "password" field with "***"
// before a body is recorded on a span. Non-JSON or password-free bodies
// pass through unchanged.
// Middleware creates a span per request. Span name is the ServeMux route
// pattern (e.g. "GET /api/weddings/{wid}/guests"), set at request end when
// the mux has populated r.Pattern; fallback "METHOD path". With
// OTEL_LOG_BODY=all, JSON request/response bodies are recorded on the span
// (capped at 8KB). SSE endpoints are never body-captured.
func Middleware(next http.Handler) http.Handler {
	logBody := os.Getenv("OTEL_LOG_BODY") == "all"
	tracer := otel.Tracer(tracerName)
	prop := otel.GetTextMapPropagator()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		reqAttrs := []attribute.KeyValue{
			attribute.String("http.request.path", r.URL.Path),
		}
		// Client address comes from RemoteAddr, which ProxyAwareMiddleware
		// (registered outside this middleware) has already rewritten from
		// X-Forwarded-For. That rewrite strips the port, so client.port is
		// only recorded when a numeric port is present.
		if host, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			reqAttrs = append(reqAttrs, semconv.ClientAddress(host))
			if p, err := strconv.Atoi(port); err == nil {
				reqAttrs = append(reqAttrs, semconv.ClientPort(p))
			}
		} else if r.RemoteAddr != "" {
			// Bare address (no port): record as-is.
			reqAttrs = append(reqAttrs, semconv.ClientAddress(r.RemoteAddr))
		}
		if logBody && !strings.Contains(r.URL.Path, "/events") && isJSONContentType(r.Header.Get("Content-Type")) {
			body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyLog+1))
			r.Body = io.NopCloser(bytes.NewReader(body))
			addBodyEvent(span, "http.request.body", body)
		}

		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(ctx))

		if r.Pattern != "" {
			span.SetName(r.Pattern)
			reqAttrs = append(reqAttrs, attribute.String("http.route", r.Pattern))
		}
		span.SetAttributes(reqAttrs...)
		span.SetAttributes(attribute.Int("http.response.status_code", rec.status))
		if logBody && rec.buf.Len() > 0 && isJSONContentType(rec.Header().Get("Content-Type")) {
			addBodyEvent(span, "http.response.body", rec.buf.Bytes())
		}
		if rec.status >= 400 {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
			span.SetAttributes(attribute.Bool("http.error", true))
		}
	})
}

// maskPassword replaces the top-level JSON "password" field with "***"
// before a body is recorded on a span. Non-JSON or password-free bodies
// pass through unchanged.
func maskPassword(body []byte) string {
	if !strings.Contains(strings.ToLower(string(body)), "password") {
		return string(body)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return string(body)
	}
	masked := false
	for k := range m {
		if strings.EqualFold(k, "password") {
			m[k] = "***"
			masked = true
		}
	}
	if !masked {
		return string(body)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return string(body)
	}
	return string(out)
}

func isJSONContentType(ct string) bool {
	return strings.Contains(ct, "json")
}

// recorder captures the response status and written bytes for the span.
type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.buf.Len() < maxBodyLog {
		r.buf.Write(b)
	}
	return r.ResponseWriter.Write(b)
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
	if p.logSQL && db.Statement.SQL.Len() > 0 {
		span.AddEvent("db.query", trace.WithAttributes(semconv.DBQueryText(db.Statement.SQL.String())))
	}
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
