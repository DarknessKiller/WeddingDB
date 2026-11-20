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

// requestReadTimeout bounds how long one request may spend sending its body.
// Zero would let a client trickle bytes forever and hold a connection; 5
// minutes still fits a 1 MiB upload on a bad mobile link.
const requestReadTimeout = 5 * time.Minute

// NewFuegoServer builds the server. opts are appended to the defaults; global
// middlewares must come in this way, since they wrap the router itself.
func NewFuegoServer(env Env, version string, opts ...fuego.ServerOption) *fuego.Server {
	s := fuego.NewServer(append([]fuego.ServerOption{
		fuego.WithAddr(":" + env.Port),
		// Direct deployments have no reverse proxy to cap request bodies; 1 MiB
		// covers every JSON payload this API accepts.
		fuego.WithMaxBodySize(1024 * 1024),
		fuego.WithEngineOptions(
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
			fuego.WithErrorHandler(internalErrorHandler),
		),
	}, opts...)...,
	)

	// SSE streams must stay open indefinitely; WriteTimeout is a whole-response
	// deadline and fuego's 30s default would kill every stream at 30s, forcing
	// a reconnect/resync loop. Keepalive pings cannot extend it.
	s.WriteTimeout = 0
	// Large/slow uploads need more than fuego's 30s ReadTimeout default, but it
	// must stay bounded: an unbounded read lets a client hold the connection
	// open by trickling a body. ReadHeaderTimeout still guards slowloris
	// headers, and the body size is capped above.
	s.ReadTimeout = requestReadTimeout

	// Set the public server URL in the OpenAPI spec
	if env.PublicURL != "" {
		spec := s.OutputOpenAPISpec()
		spec.Servers = openapi3.Servers{
			&openapi3.Server{URL: env.PublicURL},
		}
	}

	return s
}

// internalErrorHandler keeps unexpected errors out of responses: fuego's
// default serializes the raw error, leaking Postgres schema and query detail
// on 500s.
func internalErrorHandler(ctx context.Context, err error) error {
	var httpErr fuego.HTTPError
	var statusErr fuego.ErrorWithStatus
	if errors.As(err, &httpErr) || errors.As(err, &statusErr) {
		return fuego.HandleHTTPError(ctx, err)
	}
	slog.ErrorContext(ctx, "unhandled error", "error", err)
	return fuego.HTTPError{Status: http.StatusInternalServerError, Title: "Internal error"}
}
