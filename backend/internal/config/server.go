package config

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
)

func NewFuegoServer(env Env, version string) *fuego.Server {
	s := fuego.NewServer(
		fuego.WithAddr(":" + env.Port),
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
		),
	)

	// SSE streams must stay open indefinitely; WriteTimeout is a whole-response
	// deadline and fuego's 30s default would kill every stream at 30s, forcing
	// a reconnect/resync loop. Keepalive pings cannot extend it.
	s.Server.WriteTimeout = 0

	// Set the public server URL in the OpenAPI spec
	if env.PublicURL != "" {
		spec := s.Engine.OutputOpenAPISpec()
		spec.Servers = openapi3.Servers{
			&openapi3.Server{URL: env.PublicURL},
		}
	}

	return s
}
