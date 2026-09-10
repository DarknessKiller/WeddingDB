package main

import (
	"context"
	"log"
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

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		app.SSEHub.Shutdown()
		if app.ShutdownTracing != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.ShutdownTracing(ctx); err != nil {
				log.Println("Warning: tracing shutdown:", err)
			}
		}
		os.Exit(0)
	}()

	log.Printf("Server starting on :%s (v%s)", env.Port, version)
	if err := app.Server.Run(); err != nil {
		log.Fatal(err)
	}
}
