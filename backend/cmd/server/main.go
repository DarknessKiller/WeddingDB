package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
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
		os.Exit(0)
	}()

	log.Printf("Server starting on :%s (v%s)", env.Port, version)
	if err := app.Server.Run(); err != nil {
		log.Fatal(err)
	}
}
