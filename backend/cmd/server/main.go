package main

import (
	"log"
	"weddingdb/internal/bootstrap"
	"weddingdb/internal/config"
)

func main() {
	env := config.LoadEnv()
	app := bootstrap.Init(env)

	log.Printf("Server starting on :%s", env.Port)
	if err := app.Server.Run(); err != nil {
		log.Fatal(err)
	}
}
