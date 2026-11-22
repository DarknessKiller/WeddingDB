package config

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
)

// maxBodyBytes is the JSON and form body limit; uploads keep their own
// per-file limit.
const maxBodyBytes = 1024 * 1024

func NewFuegoServer(env Env, version string) *fuego.Server {
	s := fuego.NewServer(
		fuego.WithAddr(":"+env.Port),
		// Bounds every body Fuego reads, not only the JSON bodies the route
		// middleware checks, so a huge text/plain or octet-stream POST cannot
		// exhaust memory before auth or rate limiting runs.
		fuego.WithMaxBodySize(maxBodyBytes),
		fuego.WithEngineOptions(
			fuego.WithErrorHandler(func(ctx context.Context, err error) error {
				if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
					return fuego.HTTPError{Status: http.StatusRequestEntityTooLarge, Title: "Request body too large"}
				}
				if _, ok := errors.AsType[fuego.ErrorWithStatus](err); ok {
					return fuego.ErrorHandler(ctx, err)
				}
				slog.ErrorContext(ctx, "request failed", "error", err)
				return fuego.InternalServerError{Title: "Internal Server Error"}
			}),
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
				Info: &openapi3.Info{
					Title:       "WeddingDB API",
					Description: "Wedding guest management and seating API",
					Version:     version,
				},
				UIHandler: func(specURL string) http.Handler {
					return fuego.DefaultOpenAPIHandler(specURL)
				},
			}),
		),
	)

	// SSE streams must stay open indefinitely; WriteTimeout is a whole-response
	// deadline and fuego's 30s default would kill every stream at 30s, forcing
	// a reconnect/resync loop. Keepalive pings cannot extend it.
	s.WriteTimeout = 0
	// Allow large uploads while bounding request duration.
	s.ReadTimeout = 5 * time.Minute

	// Set the public server URL in the OpenAPI spec
	if env.PublicURL != "" {
		spec := s.OutputOpenAPISpec()
		spec.Servers = openapi3.Servers{
			&openapi3.Server{URL: env.PublicURL},
		}
	}

	return s
}
