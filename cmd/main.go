//go:build linux

package main

import (
	"context"

	"github.com/amitschendel/curing/pkg/config"
	"github.com/amitschendel/curing/pkg/executer"
)

func main() {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	e, err := executer.NewExecuter(cfg, ctx)
	if err != nil {
		panic(err)
	}

	// Set the commands channel
	e.SetCommandsChannel(make(chan string)) // TODO: put the real channel here.

	// Start the executer (this is a blocking call)
	e.Run()
}
