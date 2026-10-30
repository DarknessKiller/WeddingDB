package config

import "github.com/go-fuego/fuego"

func NewFuegoServer(env Env) *fuego.Server {
	s := fuego.NewServer(
		fuego.WithAddr(":"+env.Port),
		fuego.WithEngineOptions(
			// The API surface is not published: no swagger UI, no spec route.
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{Disabled: true}),
		),
	)

	// SSE streams must stay open indefinitely; WriteTimeout is a whole-response
	// deadline and fuego's 30s default would kill every stream at 30s, forcing
	// a reconnect/resync loop. Keepalive pings cannot extend it.
	s.Server.WriteTimeout = 0
	// Large/slow uploads must not be reset mid-transfer (fuego defaults
	// ReadTimeout to 30s). middleware.LimitBody caps the body; ReadHeaderTimeout
	// still applies so slowloris headers die.
	s.Server.ReadTimeout = 0

	return s
}
