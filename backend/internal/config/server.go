package config

import (
	"github.com/go-fuego/fuego"
)

func NewFuegoServer(env Env) *fuego.Server {
	return fuego.NewServer(
		fuego.WithAddr(":" + env.Port),
	)
}
