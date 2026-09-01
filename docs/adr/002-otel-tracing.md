# ADR 002: Opt-in OpenTelemetry tracing

Date: 2026-09-01

## Context

Debugging request issues needs visibility across frontend → backend → DB. Tracing must stay off by default (single-night event, low footprint) and must not hurt when the collector is down. SvelteKit frontend can send W3C `traceparent` headers.

## Decision

- Opt-in via `OTEL_ENABLED=true`. Off = zero cost (no exporter, no provider, no middleware).
- OTLP HTTP exporter to `http://localhost:4318/v1/traces` (overridable via `OTEL_EXPORTER_OTLP_ENDPOINT`), Jaeger all-in-one in compose behind profile `tracing`.
- Drop-on-failure: exporter retries disabled; batch processor queue is bounded (2048 spans) and drops when full. Down Jaeger costs nothing but lost spans, no memory balloon.
- Parent-based AlwaysSample: root spans sample, upstream `traceparent` decides continuation. W3C tracecontext only.
- Hand-rolled net/http middleware (not otelhttp, one dep less) registered before CORS/proxy middleware so preflights trace too. Span named by route pattern (`GET /api/weddings/{wid}/guests`) set post-dispatch, fallback `METHOD path`.
- Optional body capture with `OTEL_LOG_BODY=all`: JSON request/response bodies recorded on the span, capped at 8KB, SSE endpoints excluded. Bodies can contain passwords; off by default.
- GORM callback plugin: `gorm.query` span per query with `db.system.name=postgresql`, rows affected, status. Statement text only with `OTEL_LOG_SQL=all` (SQL can contain PII).
- Retention intent is 1 month; compose Jaeger uses memory storage and does not retain. Production storage choice deferred — Jaeger with badger storage or ELGC is the upgrade path.

## Alternatives considered

- Contrib GORM instrumentation package and otelhttp: extra dependencies for ~40 lines of callback/handler code each, hand-rolled instead.
- Retry with backoff: keeps queue when collector blips, but a down collector balloons memory. Drop-on-failure chosen.
- Always-on tracing: event runs one night, cost not justified without opt-in.
- Full semconv HTTP attributes on every span: noise; route-pattern name + defaults suffice.

## Consequences

- New env vars: `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT` (optional), `OTEL_LOG_SQL` (optional), `OTEL_LOG_BODY` (optional).
- Tracing shutdown flushes on SIGINT/SIGTERM before exit.
- Memory storage loses all spans on Jaeger restart; fine for local debugging, revisit for production.

## Status

Accepted.
