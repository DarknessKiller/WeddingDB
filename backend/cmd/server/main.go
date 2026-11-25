package main

import (
	"cmp"
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
	// The runtime image is distroless (no curl), so the compose healthcheck runs the binary itself.
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		c := http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get("http://127.0.0.1:" + cmp.Or(os.Getenv("PORT"), "8080") + "/health")
		if err != nil || resp.StatusCode != http.StatusNoContent {
			os.Exit(1)
		}
		os.Exit(0)
	}

	env := config.LoadEnv()
	app := bootstrap.Init(env, version)

	shutdownDone := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		// End SSE streams first so they do not hold up the drain, but keep the
		// hub publishing until the server stops: writes committed during the
		// drain must still reach other replicas.
		app.SSEHub.CloseStreams()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Server.Shutdown(ctx); err != nil {
			log.Println("Warning: server shutdown:", err)
		}
		hubCtx, hubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer hubCancel()
		app.SSEHub.Shutdown(hubCtx)
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
