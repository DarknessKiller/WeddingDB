package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"
)

var version = "dev"

// shutdownTimeout stays under Docker's default 10s stop grace period, so
// in-flight requests finish before the container is killed.
const shutdownTimeout = 8 * time.Second

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env, version)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Server starting on :%s (v%s)", env.Port, version)
		serveErr <- app.Server.Run()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	case <-ctx.Done():
	}

	// Close SSE streams first: they never end on their own, and Shutdown
	// waits for every open request.
	app.SSEHub.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		log.Println("Warning: HTTP shutdown:", err)
	}
	if app.ShutdownTracing != nil {
		if err := app.ShutdownTracing(shutdownCtx); err != nil {
			log.Println("Warning: tracing shutdown:", err)
		}
	}
}
