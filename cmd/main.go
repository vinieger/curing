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

	executer, err := executer.NewExecuter(cfg, ctx)
	if err != nil {
		panic(err)
	}

	// Start the executer (this is a blocking call)
	executer.Run()
}
