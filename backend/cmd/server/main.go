package main

import (
	"log"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"
)

var version = "dev"

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env, version)

	log.Printf("Server starting on :%s (v%s)", env.Port, version)
	if err := app.Server.Run(); err != nil {
		log.Fatal(err)
	}
}
