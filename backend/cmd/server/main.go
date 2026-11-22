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

const shutdownTimeout = 10 * time.Second

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env, version)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		// Close streams first: http.Server.Shutdown waits for handlers, and an
		// open SSE stream would otherwise hold the drain until the client leaves.
		app.SSEHub.CloseClients()
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelDrain()
		if err := app.Server.Shutdown(drainCtx); err != nil {
			log.Println("Warning: server shutdown:", err)
		}
		app.SSEHub.Shutdown()
		if app.ShutdownTracing != nil {
			traceCtx, cancelTrace := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelTrace()
			if err := app.ShutdownTracing(traceCtx); err != nil {
				log.Println("Warning: tracing shutdown:", err)
			}
		}
		if sqlDB, err := app.DB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				log.Println("Warning: closing database:", err)
			}
		}
		if err := app.Redis.Close(); err != nil {
			log.Println("Warning: closing redis:", err)
		}
	}()

	log.Printf("Server starting on :%s (v%s)", env.Port, version)
	// Run blocks until Shutdown closes the listener after SIGINT/SIGTERM.
	if err := app.Server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-drained
}
