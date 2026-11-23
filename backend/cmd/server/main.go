package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"
)

var version = "dev"

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env, version)

	shutdownDone := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		// Stop SSE first so its long-lived streams do not hold up the drain.
		app.SSEHub.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Server.Shutdown(ctx); err != nil {
			log.Println("Warning: server shutdown:", err)
		}
		close(shutdownDone)
	}()

	log.Printf("Server starting on :%s (v%s)", env.Port, version)
	if err := app.Server.Run(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		// Shutdown closes the listener, so Serve returns before in-flight
		// requests finish draining.
		<-shutdownDone
	}
	if app.ShutdownTracing != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.ShutdownTracing(ctx); err != nil {
			log.Println("Warning: tracing shutdown:", err)
		}
	}
}
