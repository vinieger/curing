package main

import (
	"github.com/amitschendel/curing/pkg/config"
	"github.com/amitschendel/curing/pkg/server"
)

func main() {
	cfg, err := config.LoadConfig("cmd/config.json")
	if err != nil {
		panic(err)
	}
	s := server.NewServer(cfg.Server.Port)
	s.Run()
}
